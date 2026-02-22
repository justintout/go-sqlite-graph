# go-sqlite-graph

Pure Go labeled property graph database backed by SQLite. No CGo required.

Store nodes and edges with typed columns and arbitrary JSON properties, then traverse relationships using a fluent Go query builder that compiles to recursive CTEs for multi-hop path queries.

```go
g, _ := graph.Open("file:my.db", nil)
defer g.Close()

// Find friends of friends within 3 hops
results, _ := g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 3).
    Run(ctx)

for _, n := range results.Nodes() {
    fmt.Println(n.Name, n.Properties)
}
```

## Install

```
go get github.com/justintout/go-sqlite-graph
```

Requires Go 1.23+. The only dependency is `zombiezen.com/go/sqlite`.

## Schema

The library manages three tables automatically on `Open()`:

| Table | Purpose |
|---|---|
| `nodes` | `id`, `name`, `created_at`, `updated_at`, `properties` (JSON) |
| `node_labels` | `node_id`, `label` — multiple labels per node |
| `edges` | `id`, `source_id`, `target_id`, `type`, `name`, `created_at`, `updated_at`, `properties` (JSON) |

Schema is versioned with `sqlitemigration` and applied automatically by default. Pass `AutoMigrate: false` in options and call `g.Migrate(ctx)` manually for more control.

## Go API

### Opening a graph

```go
// Auto-migrate schema (default).
g, err := graph.Open("file:my.db", nil)

// Manual migration.
f := false
g, err := graph.Open("file:my.db", &graph.Options{AutoMigrate: &f})
g.Migrate(ctx)
```

### Nodes

```go
// Create a node with multiple labels and JSON properties.
alice := &graph.Node{
    Name:       "Alice",
    Labels:     []string{"Person", "Engineer"},
    Properties: map[string]any{"age": 30, "city": "NYC"},
}
g.CreateNode(ctx, alice) // alice.ID is set after create

// Read, update, delete.
node, err := g.GetNode(ctx, alice.ID)
alice.Name = "Alice Smith"
g.UpdateNode(ctx, alice)
g.DeleteNode(ctx, alice.ID) // cascades to labels and edges

// Manage labels independently.
g.AddLabels(ctx, alice.ID, "Manager")
g.RemoveLabels(ctx, alice.ID, "Engineer")
```

### Edges

```go
edge := &graph.Edge{
    SourceID:   alice.ID,
    TargetID:   bob.ID,
    Type:       "KNOWS",
    Properties: map[string]any{"since": 2020},
}
g.CreateEdge(ctx, edge)

e, err := g.GetEdge(ctx, edge.ID)
g.UpdateEdge(ctx, edge)
g.DeleteEdge(ctx, edge.ID)
```

### Transactions

```go
tx, err := g.BeginTx(ctx)
defer tx.Rollback() // no-op if already committed

tx.CreateNode(ctx, node)
tx.CreateEdge(ctx, edge)

err = tx.Commit()
```

All CRUD methods are available on both `*Graph` and `*Tx`.

### Query builder

The query builder compiles to SQL. Simple matches use JOINs; multi-hop traversals use recursive CTEs. Cycles are handled safely via `UNION` deduplication with a configurable depth cap (max 10 hops).

```go
// Match by label.
g.Match("Person").Run(ctx)

// Filter on typed columns.
g.Match("Person").Where("name", "=", "Alice").Run(ctx)

// Filter on JSON properties.
g.Match("Person").WhereJSON("age", ">", 30).Run(ctx)

// Single-hop traversal (compiles to JOIN).
g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 1).
    Run(ctx)

// Multi-hop traversal (compiles to recursive CTE).
g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 3).
    Run(ctx)

// Chained traversals.
g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 2).
    Related("WORKS_AT", 1, 1).
    WhereRel("name", "=", "Acme").
    Run(ctx)

// Direction control.
g.Match("Person").
    Where("name", "=", "Bob").
    RelatedDir("KNOWS", graph.Incoming, 1, 1). // who knows Bob?
    Run(ctx)

// Count and pagination.
count, _ := g.Match("Person").Count(ctx)
results, _ := g.Match("Person").Limit(10).Offset(20).Run(ctx)
```

### Results

```go
res, _ := g.Match("Person").WhereJSON("age", ">", 25).Run(ctx)

// Iterate.
for res.Next() {
    row := res.Row()
    fmt.Println(row.Node.Name, row.Node.Properties)
}

// Or get all at once.
nodes := res.Nodes()
count := res.Len()
```

## Design

- **Recursive CTEs** for variable-length path queries, with `UNION` to prevent cycles and a hard cap of 10 hops.
- **Labeled property graph**: nodes support multiple labels (stored in a separate table), edges have a single directed type.
- **Typed columns + JSON overflow**: `id`, `name`, and timestamps are real columns; arbitrary properties live in a JSON column queryable via SQLite's `->>'$.path'` operator.
- **Connection pooling** via `sqlitex.Pool` / `sqlitemigration.Pool` for concurrent access.
- **Single package**: everything lives in `package graph` at the module root.

## Benchmarks

```
go test -bench=. -benchmem ./...
```

Results on Apple M3 Max:

```
BenchmarkTraversalChain/hops=1      37976     31510 ns/op     2707 B/op     61 allocs/op
BenchmarkTraversalChain/hops=2      15195     79016 ns/op     4170 B/op     70 allocs/op
BenchmarkTraversalChain/hops=3      12860     93882 ns/op     4917 B/op     86 allocs/op
BenchmarkTraversalChain/hops=5      10000    115139 ns/op     6408 B/op    117 allocs/op
BenchmarkTraversalChain/hops=10      6820    175823 ns/op    10102 B/op    194 allocs/op

BenchmarkTraversalFanout/depth=2    18614     64336 ns/op    11518 B/op    217 allocs/op  (13 nodes)
BenchmarkTraversalFanout/depth=4      926   1307569 ns/op    92186 B/op   1840 allocs/op  (121 nodes)
BenchmarkTraversalFanout/depth=6       12  93185375 ns/op   828125 B/op  16426 allocs/op  (1093 nodes)

BenchmarkTraversalDense/hops=1       3205    378662 ns/op     5734 B/op    129 allocs/op  (500 nodes, 5 edges/node)
BenchmarkTraversalDense/hops=2        220   5481183 ns/op    25130 B/op    524 allocs/op
BenchmarkTraversalDense/hops=3         45  25483407 ns/op   104028 B/op   2218 allocs/op
BenchmarkTraversalDense/hops=5          5 209771308 ns/op   366190 B/op   7815 allocs/op
BenchmarkTraversalDense/hops=10         2 931694438 ns/op   375852 B/op   8013 allocs/op

BenchmarkCreateNode                 78876     15556 ns/op     1817 B/op     38 allocs/op
BenchmarkCreateEdge                 83337     15518 ns/op      818 B/op     19 allocs/op
BenchmarkBulkInsertTx/batch=10       6548    204779 ns/op    14577 B/op    382 allocs/op
BenchmarkBulkInsertTx/batch=100       715   1800590 ns/op   141614 B/op   3801 allocs/op
BenchmarkBulkInsertTx/batch=1000       70  16154103 ns/op  1437031 B/op  39484 allocs/op

BenchmarkMatchSimple/nodes=100      51266     23259 ns/op     1525 B/op     29 allocs/op
BenchmarkMatchSimple/nodes=1000      7406    164950 ns/op     1523 B/op     29 allocs/op
BenchmarkMatchSimple/nodes=10000      786   1526852 ns/op     1526 B/op     29 allocs/op
```

Key takeaways:
- **Chain traversal** scales linearly with hop depth (~31us at 1 hop to ~176us at 10 hops on a 100-node chain).
- **Fanout traversal** scales with the number of nodes reached; a depth-6 tree with fanout 3 (1093 nodes) takes ~93ms.
- **Dense graph traversal** is the most expensive — on a 500-node graph with 5 edges/node, the reachable set explodes quickly. The `UNION` deduplication in the CTE prevents infinite loops but can't prevent visiting the full reachable set.
- **Simple match** scales linearly with table size (SQLite index scan).
- **Bulk insert** throughput is ~16ms for 1000 nodes+edges in a single transaction (~62k inserts/sec).

## License

BSD-3-Clause

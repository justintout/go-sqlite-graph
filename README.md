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

The query builder compiles to SQL. Each traversal step compiles to a CTE of node IDs; multi-hop steps use recursive CTEs. Cycles are handled safely via `UNION` deduplication with a configurable depth cap (max 10 hops).

```go
// Match by label.
g.Match("Person").Run(ctx)

// Filter on typed columns.
g.Match("Person").Where("name", "=", "Alice").Run(ctx)

// Filter on JSON properties.
g.Match("Person").WhereJSON("age", ">", 30).Run(ctx)

// Single-hop traversal.
g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 1).
    Run(ctx)

// Multi-hop traversal (recursive CTE).
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

Results on Apple M3 Max, in-memory database unless noted:

```
BenchmarkTraversalChain/hops=1          8.8µs      2804 B/op     50 allocs/op  (100-node chain)
BenchmarkTraversalChain/hops=10        32.6µs      9542 B/op    195 allocs/op

BenchmarkTraversalFanout/depth=2       33.6µs     10956 B/op    218 allocs/op  (13 nodes)
BenchmarkTraversalFanout/depth=4      195.5µs     91629 B/op   1841 allocs/op  (121 nodes)
BenchmarkTraversalFanout/depth=6       1.88ms    827562 B/op  16426 allocs/op  (1093 nodes)

BenchmarkTraversalDense/hops=1         16.6µs      5850 B/op    118 allocs/op  (500 nodes, 5 edges/node)
BenchmarkTraversalDense/hops=3        278.2µs    103407 B/op   2218 allocs/op
BenchmarkTraversalDense/hops=10        9.79ms    374402 B/op   8008 allocs/op

BenchmarkTraversalLarge/out/hops=1     14.0µs      4322 B/op     85 allocs/op  (10k nodes, 3 edges/node)
BenchmarkTraversalLarge/out/hops=3     99.9µs     31605 B/op    670 allocs/op
BenchmarkTraversalLarge/out/hops=6     2.55ms    759353 B/op  16290 allocs/op
BenchmarkTraversalLarge/both/hops=1    21.1µs      6650 B/op    138 allocs/op
BenchmarkTraversalLarge/both/hops=3   487.2µs    165757 B/op   3523 allocs/op

BenchmarkMatchSimple/nodes=100          4.2µs      1623 B/op     31 allocs/op
BenchmarkMatchSimple/nodes=10000        4.6µs      1623 B/op     31 allocs/op

BenchmarkCreateNode                    16.2µs      1802 B/op     36 allocs/op
BenchmarkCreateNodeFile                76.4µs      1808 B/op     35 allocs/op  (on disk, WAL)
BenchmarkCreateEdge                    10.9µs       802 B/op     17 allocs/op
BenchmarkBulkInsertTx/batch=1000      18.32ms   1406150 B/op  35446 allocs/op
```

Key takeaways:
- **Traversal cost tracks the reached set**, not the graph size. Each hop is a covering-index lookup, so one hop costs about the same on a 100-node chain and a 10k-node graph.
- **Dense graph traversal** is the most expensive. `UNION` deduplicates on (node, depth), so a node reachable at several depths is expanded once per depth.
- **Result decoding** is a large share of big traversals: each node's JSON properties are unmarshaled into a map.
- **Name matches** use an index and do not grow with label size.

## License

BSD-3-Clause

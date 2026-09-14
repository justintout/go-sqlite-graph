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

// Project columns and property paths; unreturned fields stay zero.
g.Match("Person").
    Related("KNOWS", 1, 3).
    Return("name", "age").
    Run(ctx)

// Count and pagination.
count, _ := g.Match("Person").Count(ctx)
results, _ := g.Match("Person").Limit(10).Offset(20).Run(ctx)
```

`Where` and `WhereRel` accept the node columns `id`, `name`, `created_at`, `updated_at`, and `properties`. Filter properties with `WhereJSON` and `WhereRelJSON`.

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
BenchmarkTraversalChain/hops=1          7.6µs      2220 B/op     33 allocs/op  (100-node chain)
BenchmarkTraversalChain/hops=10        31.4µs      8889 B/op    173 allocs/op

BenchmarkTraversalFanout/depth=2       33.1µs     10417 B/op    201 allocs/op  (13 nodes)
BenchmarkTraversalFanout/depth=4      199.0µs     90999 B/op   1824 allocs/op  (121 nodes)
BenchmarkTraversalFanout/depth=6       1.92ms    826277 B/op  16408 allocs/op  (1093 nodes)

BenchmarkTraversalDense/hops=1         16.2µs      5263 B/op    101 allocs/op  (500 nodes, 5 edges/node)
BenchmarkTraversalDense/hops=3        279.0µs    102656 B/op   2196 allocs/op
BenchmarkTraversalDense/hops=10        9.72ms    373273 B/op   7985 allocs/op

BenchmarkTraversalLarge/out/hops=1     13.0µs      3738 B/op     68 allocs/op  (10k nodes, 3 edges/node)
BenchmarkTraversalLarge/out/hops=3     98.8µs     30919 B/op    648 allocs/op
BenchmarkTraversalLarge/out/hops=6     2.55ms    757988 B/op  16267 allocs/op
BenchmarkTraversalLarge/both/hops=1    23.6µs      6007 B/op    118 allocs/op
BenchmarkTraversalLarge/both/hops=3   484.2µs    164892 B/op   3498 allocs/op

BenchmarkMatchSimple/nodes=100          4.1µs      1619 B/op     22 allocs/op
BenchmarkMatchSimple/nodes=10000        4.4µs      1619 B/op     22 allocs/op

BenchmarkCreateNode                    11.6µs      1679 B/op     36 allocs/op
BenchmarkCreateNodeFile                67.5µs      1686 B/op     35 allocs/op  (on disk, WAL)
BenchmarkCreateEdge                     7.2µs       714 B/op     17 allocs/op
BenchmarkBulkInsertTx/batch=1000       6.47ms    835897 B/op  29476 allocs/op
```

Key takeaways:
- **Traversal cost tracks the reached set**, not the graph size. Each hop is a covering-index lookup, so one hop costs about the same on a 100-node chain and a 10k-node graph.
- **Dense graph traversal** is the most expensive. `UNION` deduplicates on (node, depth), so a node reachable at several depths is expanded once per depth.
- **Result decoding** is a large share of big traversals: each node's JSON properties are unmarshaled into a map. `Return` skips it; returning only `name` from a 6-hop traversal with 10-key properties cuts time from 6.6ms to 1.8ms.

[`bench/compare`](bench/compare) benchmarks against [GraphQLite](https://github.com/colliery-io/graphqlite) on 10k-node random and 100k-node power-law graphs.
- **Name matches** use an index and do not grow with label size.

## License

BSD-3-Clause

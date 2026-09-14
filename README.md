# go-sqlite-graph

[![test](https://github.com/justintout/go-sqlite-graph/actions/workflows/test.yml/badge.svg)](https://github.com/justintout/go-sqlite-graph/actions/workflows/test.yml)

A labeled property graph database for Go, stored in SQLite. It uses the pure-Go `modernc.org/sqlite` driver, so it needs no CGo.

Nodes and edges have fixed columns plus a JSON properties document. A query builder compiles traversals to SQL, using recursive CTEs for multi-hop paths.

```go
g, _ := graph.Open("file:my.db", nil)
defer g.Close()

// Find people Alice knows within 3 hops.
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

Requires Go 1.23 or later. The `graph` package depends only on `zombiezen.com/go/sqlite`. The [`viz`](viz) subpackage renders nodes and edges as interactive HTML and adds `go-echarts`.

## Schema

`Open` creates and migrates three tables:

| Table | Columns |
|---|---|
| `nodes` | `id`, `name`, `created_at`, `updated_at`, `properties` (JSON) |
| `node_labels` | `node_id`, `label`; a node can have many labels |
| `edges` | `id`, `source_id`, `target_id`, `type`, `name`, `created_at`, `updated_at`, `properties` (JSON) |

`sqlitemigration` versions the schema. Deleting a node deletes its labels and edges through foreign key cascades. Timestamps are UTC strings in `2006-01-02T15:04:05.000Z` format.

## Go API

### Opening a graph

```go
// Migrate the schema on first use (default).
g, err := graph.Open("file:my.db", nil)

// Migrate explicitly.
f := false
g, err := graph.Open("file:my.db", &graph.Options{AutoMigrate: &f, PoolSize: 4})
err = g.Migrate(ctx)
```

`Open` takes a SQLite URI. Every connection opens in WAL mode with foreign keys on. `PoolSize` defaults to 10.

### Nodes

```go
alice := &graph.Node{
    Name:       "Alice",
    Labels:     []string{"Person", "Engineer"},
    Properties: map[string]any{"age": 30, "city": "NYC"},
}
g.CreateNode(ctx, alice) // sets alice.ID, CreatedAt, and UpdatedAt

node, err := g.GetNode(ctx, alice.ID)

alice.Name = "Alice Smith"
g.UpdateNode(ctx, alice)

g.DeleteNode(ctx, alice.ID)

g.AddLabels(ctx, alice.ID, "Manager")
g.RemoveLabels(ctx, alice.ID, "Engineer")
```

`UpdateNode` replaces the name, labels, and properties with the values on the struct. A `Node` with nil `Labels` loses all its labels. Read the node with `GetNode` before changing one field.

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
g.UpdateEdge(ctx, edge) // replaces type, name, and properties
g.DeleteEdge(ctx, edge.ID)
```

Edges are directed. `UpdateEdge` does not change the endpoints.

Get, update, and delete return an error when the ID does not exist.

### Transactions

```go
tx, err := g.BeginTx(ctx)
defer tx.Rollback() // does nothing after Commit

tx.CreateNode(ctx, node)
tx.CreateEdge(ctx, edge)
tx.Match("Person").Count(ctx)

err = tx.Commit()
```

`*Tx` has every CRUD method on `*Graph`, and `Match`. `BeginTx` takes the write lock immediately, so only one transaction writes at a time. Outside a transaction, each write is atomic. Inside one, a method that returns an error can leave part of its writes behind, such as a node without its labels. Roll back after any error.

Load large batches in one transaction. On an in-memory database, 1,000 nodes and 999 edges take about 19ms as separate writes and 6.5ms in one transaction. The gap is larger on disk, where each write outside a transaction is its own commit.

### Queries

`Match` selects nodes by label. Each `Related` step replaces the current node set with the nodes it reaches. The result holds the distinct nodes of the last step, in no guaranteed order.

```go
// Match by label.
g.Match("Person").Run(ctx)

// Filter on node columns.
g.Match("Person").Where("name", "=", "Alice").Run(ctx)

// Filter on JSON properties. Nested paths use dots.
g.Match("Person").WhereJSON("age", ">", 30).Run(ctx)
g.Match("Person").WhereJSON("address.city", "=", "NYC").Run(ctx)

// One hop.
g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 1).
    Run(ctx)

// One to three hops.
g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 3).
    Run(ctx)

// Chained steps. WhereRel filters the nodes the last step reached.
g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 2).
    Related("WORKS_AT", 1, 1).
    WhereRel("name", "=", "Acme").
    Run(ctx)

// Direction: Outgoing (the Related default), Incoming, or Both.
g.Match("Person").
    Where("name", "=", "Bob").
    RelatedDir("KNOWS", graph.Incoming, 1, 1). // who knows Bob?
    Run(ctx)

// Expand each reached node once. See "Breadth-first steps" below.
g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 6).
    BreadthFirst().
    Run(ctx)

// Return only some columns and property paths.
g.Match("Person").
    Related("KNOWS", 1, 3).
    Return("name", "age").
    Run(ctx)

// Count and paginate.
count, _ := g.Match("Person").Count(ctx)
results, _ := g.Match("Person").Limit(10).Offset(20).Run(ctx)
```

`Where` and `WhereRel` accept the columns `id`, `name`, `created_at`, `updated_at`, and `properties`. `WhereJSON` and `WhereRelJSON` accept any property path. When the value, or the element type of an `IN` slice, is a Go integer or float, the property is cast to `INTEGER` or `REAL` before comparison.

The operators are `=`, `!=`, `<>`, `<`, `<=`, `>`, `>=`, `LIKE`, `NOT LIKE`, `IN`, `NOT IN`, `IS`, and `IS NOT`. `IN` and `NOT IN` take a slice, such as `[]string{"Alice", "Bob"}`.

A step takes between 1 and `graph.MaxHops` (10) hops. A start node is in the result only if the step reaches it, for example through a cycle. Cycles do not loop forever: a walk expands each node at most once per depth.

Query results populate the node columns and properties. They do not populate `Labels`, and they do not return edges. `Return` sets `ID` plus the named fields. It sets a property path as a key in `Properties` and leaves every other field zero. Returning fewer fields skips JSON decoding, which is a large share of the cost of big traversals.

`Count` ignores `Limit` and `Offset`.

Builder errors, such as an unknown column or a hop count out of range, are returned by `Run` or `Count`.

#### Breadth-first steps

By default, a multi-hop step expands a node once for every depth at which it is reachable. On a dense graph with cycles, most nodes are reachable at many depths. `BreadthFirst` makes the last step expand each node once. The results are the same.

`BreadthFirst` adds a fixed cost per hop level, so it is slower when the walk rarely revisits nodes. Measure your graph before using it. It requires a minimum of 1 hop.

### Results

```go
res, _ := g.Match("Person").WhereJSON("age", ">", 25).Run(ctx)

for res.Next() {
    row := res.Row()
    fmt.Println(row.Node.Name, row.Node.Properties)
}

nodes := res.Nodes()
count := res.Len()
```

`Run` reads every row before it returns.

## Design

- Multi-hop steps compile to recursive CTEs. `UNION` deduplicates on (node, depth), which stops cycles and bounds the work at each depth.
- Each step yields a set of node IDs. The query reads node rows only for the final set.
- Edge indexes cover `(source_id, type, target_id)` and `(target_id, type, source_id)`, so a hop reads only the index.
- Nodes can have many labels, stored in `node_labels`. Each edge has one type.
- `id`, `name`, and the timestamps are columns. Other properties are a JSON column, queried with SQLite's `->>` operator.
- Connections come from a `sqlitex.Pool` or `sqlitemigration.Pool`.

## Benchmarks

Traversal cost follows the number of nodes reached, not the graph size. See [BENCHMARKS.md](BENCHMARKS.md) for results and a comparison with GraphQLite.

## License

BSD-3-Clause

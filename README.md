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

## License

BSD-3-Clause

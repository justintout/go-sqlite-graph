# viz

Interactive graph visualization for [go-sqlite-graph](https://github.com/justintout/go-sqlite-graph) using [go-echarts](https://github.com/go-echarts/go-echarts).

Renders nodes and edges as self-contained HTML pages with force-directed or circular layouts, pan/zoom, drag, and labeled edges. Output goes to any `io.Writer` or can be served directly via `http.HandlerFunc`.

```go
c := viz.New(nodes, edges,
    viz.WithTitle("Social Graph"),
    viz.WithLayout(viz.ForceLayout),
)

c.Render(os.Stdout)
```

## Install

```
go get github.com/justintout/go-sqlite-graph/viz
```

## Usage

### From nodes and edges

```go
nodes := []*graph.Node{
    {ID: 1, Name: "Alice", Labels: []string{"Person"}},
    {ID: 2, Name: "Bob", Labels: []string{"Person"}},
    {ID: 3, Name: "Acme Corp", Labels: []string{"Company"}},
}

edges := []*graph.Edge{
    {SourceID: 1, TargetID: 2, Type: "KNOWS"},
    {SourceID: 1, TargetID: 3, Type: "WORKS_AT"},
}

c := viz.New(nodes, edges,
    viz.WithTitle("Social Graph"),
    viz.WithLayout(viz.ForceLayout),
)

f, _ := os.Create("graph.html")
defer f.Close()
c.Render(f)
```

### From a query result

```go
result, _ := g.Match("Person").
    Where("name", "=", "Alice").
    Related("KNOWS", 1, 3).
    Run(ctx)

c := viz.NewFromResult(result, edges)
c.Render(os.Stdout)
```

### HTTP handler

```go
c := viz.New(nodes, edges, viz.WithTitle("My Graph"))

http.HandleFunc("/graph", c.Handler())
http.ListenAndServe(":8080", nil)
```

## Options

| Option | Description | Default |
|---|---|---|
| `WithLayout(l)` | Layout algorithm: `ForceLayout` or `CircularLayout` | `ForceLayout` |
| `WithTitle(s)` | Chart title displayed above the graph | `""` |
| `WithSize(w, h)` | Chart dimensions (e.g. `"1200px"`, `"800px"`) | `"900px"` x `"500px"` |
| `WithPalette(colors)` | Custom hex colors for node categories | Neo4j-inspired palette |

## How it works

- Each node's first label determines its color category. Nodes without labels are grouped as "(unlabeled)".
- Colors cycle through the palette when there are more categories than colors.
- Edge `Type` fields are displayed as edge labels.
- Edges referencing nodes not in the provided slice are silently skipped.
- Output is a self-contained HTML page with embedded JavaScript -- no server-side rendering required.

## License

BSD-3-Clause

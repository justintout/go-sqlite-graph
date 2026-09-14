// Package viz provides interactive graph visualization using go-echarts.
package viz

import (
	"context"
	"io"
	"net/http"

	graph "github.com/justintout/go-sqlite-graph"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// Layout controls the graph layout algorithm.
type Layout string

const (
	ForceLayout    Layout = "force"
	CircularLayout Layout = "circular"
)

// Chart holds graph data and rendering configuration.
type Chart struct {
	nodes   []*graph.Node
	edges   []*graph.Edge
	layout  Layout
	title   string
	width   string
	height  string
	palette *palette
}

// Option configures a Chart.
type Option func(*Chart)

// WithLayout sets the graph layout algorithm.
func WithLayout(l Layout) Option {
	return func(c *Chart) { c.layout = l }
}

// WithTitle sets the chart title.
func WithTitle(title string) Option {
	return func(c *Chart) { c.title = title }
}

// WithSize sets the chart dimensions (e.g. "1200px", "800px").
func WithSize(width, height string) Option {
	return func(c *Chart) { c.width = width; c.height = height }
}

// WithPalette sets custom category colors.
func WithPalette(colors []string) Option {
	return func(c *Chart) { c.palette = newPalette(colors) }
}

// New creates a Chart from nodes and edges.
func New(nodes []*graph.Node, edges []*graph.Edge, options ...Option) *Chart {
	c := &Chart{
		nodes:   nodes,
		edges:   edges,
		layout:  ForceLayout,
		width:   "900px",
		height:  "500px",
		palette: newPalette(nil),
	}
	for _, opt := range options {
		opt(c)
	}
	return c
}

// EdgeReader reads the edges between a set of nodes. *graph.Graph and
// *graph.Tx implement it.
type EdgeReader interface {
	EdgesBetween(ctx context.Context, nodeIDs []int64, types ...string) ([]*graph.Edge, error)
}

// FromQuery runs q with labels selected and creates a Chart of its nodes and
// every edge between them, read from db. When q was started from a Tx, pass
// that Tx as db so the edges come from the same transaction. Use New to
// choose the nodes or edges yourself.
func FromQuery(ctx context.Context, db EdgeReader, q *graph.Query, options ...Option) (*Chart, error) {
	res, err := q.WithLabels().Run(ctx)
	if err != nil {
		return nil, err
	}
	nodes := res.Nodes()
	ids := make([]int64, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	edges, err := db.EdgesBetween(ctx, ids)
	if err != nil {
		return nil, err
	}
	return New(nodes, edges, options...), nil
}

// Render writes the chart as a self-contained HTML page to w.
func (c *Chart) Render(w io.Writer) error {
	cats, catIndex := buildCategories(c.nodes, c.palette)
	gNodes := convertNodes(c.nodes, catIndex)
	gLinks := convertEdges(c.edges, c.nodes)

	catPtrs := make([]*opts.GraphCategory, len(cats))
	for i := range cats {
		catPtrs[i] = &cats[i]
	}

	g := charts.NewGraph()
	g.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			Width:  c.width,
			Height: c.height,
		}),
		charts.WithTitleOpts(opts.Title{Title: c.title}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true)}),
	)

	graphOpts := opts.GraphChart{
		Layout:             string(c.layout),
		Roam:               opts.Bool(true),
		Draggable:          opts.Bool(true),
		FocusNodeAdjacency: opts.Bool(true),
		Categories:         catPtrs,
		EdgeLabel: &opts.EdgeLabel{
			Show:     opts.Bool(true),
			Position: "middle",
		},
	}
	if c.layout == ForceLayout {
		graphOpts.Force = &opts.GraphForce{
			Repulsion:  100,
			EdgeLength: 120,
		}
	}

	g.AddSeries("graph", nil, gLinks,
		charts.WithGraphChartOpts(graphOpts),
	)
	// AddSeries accepts only opts.GraphNode, which has no id field.
	g.MultiSeries[0].Data = gNodes

	return g.Render(w)
}

// Handler returns an http.HandlerFunc that renders the chart as HTML.
func (c *Chart) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		c.Render(w)
	}
}

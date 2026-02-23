// Package viz provides interactive graph visualization using go-echarts.
package viz

import (
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

// NewFromResult creates a Chart from a query Result and edges.
func NewFromResult(result *graph.Result, edges []*graph.Edge, options ...Option) *Chart {
	return New(result.Nodes(), edges, options...)
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

	g.AddSeries("graph", gNodes, gLinks,
		charts.WithGraphChartOpts(graphOpts),
	)

	return g.Render(w)
}

// Handler returns an http.HandlerFunc that renders the chart as HTML.
func (c *Chart) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		c.Render(w)
	}
}

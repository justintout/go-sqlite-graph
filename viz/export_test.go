package viz

import (
	graph "github.com/justintout/go-sqlite-graph"

	"github.com/go-echarts/go-echarts/v2/opts"
)

var (
	BuildCategories = buildCategories
	ConvertNodes    = convertNodes
	ConvertEdges    = convertEdges
	NodeNameIndex   = nodeNameIndex
	NewPalette      = newPalette
)

func TestableChart(nodes []*graph.Node, edges []*graph.Edge, options ...Option) *Chart {
	return New(nodes, edges, options...)
}

func ChartPalette(c *Chart) *palette {
	return c.palette
}

func PaletteColorFor(p *palette, index int) string {
	return p.colorFor(index)
}

func PaletteColors(p *palette) []string {
	return p.colors
}

type GraphCategory = opts.GraphCategory
type GraphNode = opts.GraphNode
type GraphLink = opts.GraphLink

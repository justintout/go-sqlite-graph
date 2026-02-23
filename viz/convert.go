package viz

import (
	graph "github.com/justintout/go-sqlite-graph"

	"github.com/go-echarts/go-echarts/v2/opts"
)

// buildCategories extracts unique labels from nodes and returns go-echarts
// categories with colors assigned from the palette. It also returns a map
// from label name to category index.
func buildCategories(nodes []*graph.Node, p *palette) ([]opts.GraphCategory, map[string]int) {
	catIndex := make(map[string]int)
	var cats []opts.GraphCategory

	for _, n := range nodes {
		label := "(unlabeled)"
		if len(n.Labels) > 0 {
			label = n.Labels[0]
		}
		if _, ok := catIndex[label]; !ok {
			idx := len(cats)
			catIndex[label] = idx
			cats = append(cats, opts.GraphCategory{
				Name: label,
				ItemStyle: &opts.ItemStyle{
					Color: p.colorFor(idx),
				},
			})
		}
	}
	return cats, catIndex
}

// convertNodes converts graph nodes to go-echarts GraphNode values.
func convertNodes(nodes []*graph.Node, catIndex map[string]int) []opts.GraphNode {
	result := make([]opts.GraphNode, len(nodes))
	for i, n := range nodes {
		label := "(unlabeled)"
		if len(n.Labels) > 0 {
			label = n.Labels[0]
		}
		result[i] = opts.GraphNode{
			Name:       n.Name,
			Category:   catIndex[label],
			SymbolSize: 30,
		}
	}
	return result
}

// convertEdges converts graph edges to go-echarts GraphLink values.
// Edges referencing nodes not present in the node list are silently skipped.
func convertEdges(edges []*graph.Edge, nodes []*graph.Node) []opts.GraphLink {
	nameIdx := nodeNameIndex(nodes)
	var links []opts.GraphLink
	for _, e := range edges {
		srcName, srcOK := nameIdx[e.SourceID]
		tgtName, tgtOK := nameIdx[e.TargetID]
		if !srcOK || !tgtOK {
			continue
		}
		links = append(links, opts.GraphLink{
			Source: srcName,
			Target: tgtName,
			Label: &opts.EdgeLabel{
				Show:     opts.Bool(true),
				Position: "middle",
				Formatter: e.Type,
			},
		})
	}
	return links
}

// nodeNameIndex builds a map from node ID to node Name.
func nodeNameIndex(nodes []*graph.Node) map[int64]string {
	m := make(map[int64]string, len(nodes))
	for _, n := range nodes {
		m[n.ID] = n.Name
	}
	return m
}

package viz

import (
	"strconv"

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

// graphNode adds the id echarts identifies a node by. Without an id, echarts
// identifies nodes by name: it drops every node after the first with a given
// name and attaches their edges to that first node.
type graphNode struct {
	opts.GraphNode
	ID string `json:"id"`
}

func nodeID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// convertNodes converts graph nodes to echarts graph nodes.
func convertNodes(nodes []*graph.Node, catIndex map[string]int) []graphNode {
	result := make([]graphNode, len(nodes))
	for i, n := range nodes {
		label := "(unlabeled)"
		if len(n.Labels) > 0 {
			label = n.Labels[0]
		}
		result[i] = graphNode{
			GraphNode: opts.GraphNode{
				Name:       n.Name,
				Category:   catIndex[label],
				SymbolSize: 30,
			},
			ID: nodeID(n.ID),
		}
	}
	return result
}

// convertEdges converts graph edges to go-echarts GraphLink values.
// Edges referencing nodes not present in the node list are silently skipped.
func convertEdges(edges []*graph.Edge, nodes []*graph.Node) []opts.GraphLink {
	present := make(map[int64]bool, len(nodes))
	for _, n := range nodes {
		present[n.ID] = true
	}
	var links []opts.GraphLink
	for _, e := range edges {
		if !present[e.SourceID] || !present[e.TargetID] {
			continue
		}
		links = append(links, opts.GraphLink{
			Source: nodeID(e.SourceID),
			Target: nodeID(e.TargetID),
			Label: &opts.EdgeLabel{
				Show:      opts.Bool(true),
				Position:  "middle",
				Formatter: e.Type,
			},
		})
	}
	return links
}

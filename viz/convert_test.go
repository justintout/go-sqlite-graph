package viz_test

import (
	"testing"

	graph "github.com/justintout/go-sqlite-graph"
	"github.com/justintout/go-sqlite-graph/viz"
)

func testNodes() []*graph.Node {
	return []*graph.Node{
		{ID: 1, Name: "Alice", Labels: []string{"Person"}},
		{ID: 2, Name: "Bob", Labels: []string{"Person"}},
		{ID: 3, Name: "Acme", Labels: []string{"Company"}},
	}
}

func testEdges() []*graph.Edge {
	return []*graph.Edge{
		{SourceID: 1, TargetID: 2, Type: "KNOWS"},
		{SourceID: 1, TargetID: 3, Type: "WORKS_AT"},
	}
}

func TestBuildCategories(t *testing.T) {
	nodes := testNodes()
	p := viz.NewPalette(nil)
	cats, catIndex := viz.BuildCategories(nodes, p)

	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}

	if _, ok := catIndex["Person"]; !ok {
		t.Error("missing Person category")
	}
	if _, ok := catIndex["Company"]; !ok {
		t.Error("missing Company category")
	}
	if cats[0].Name != "Person" {
		t.Errorf("expected first category Person, got %s", cats[0].Name)
	}
}

func TestBuildCategoriesUnlabeled(t *testing.T) {
	nodes := []*graph.Node{
		{ID: 1, Name: "Mystery"},
	}
	p := viz.NewPalette(nil)
	cats, catIndex := viz.BuildCategories(nodes, p)

	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	if cats[0].Name != "(unlabeled)" {
		t.Errorf("expected (unlabeled), got %s", cats[0].Name)
	}
	if _, ok := catIndex["(unlabeled)"]; !ok {
		t.Error("missing (unlabeled) category index")
	}
}

func TestConvertNodes(t *testing.T) {
	nodes := testNodes()
	p := viz.NewPalette(nil)
	_, catIndex := viz.BuildCategories(nodes, p)
	gNodes := viz.ConvertNodes(nodes, catIndex)

	if len(gNodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(gNodes))
	}
	if gNodes[0].Name != "Alice" {
		t.Errorf("expected Alice, got %s", gNodes[0].Name)
	}
	if gNodes[2].Name != "Acme" {
		t.Errorf("expected Acme, got %s", gNodes[2].Name)
	}
}

func TestConvertEdges(t *testing.T) {
	nodes := testNodes()
	edges := testEdges()
	links := viz.ConvertEdges(edges, nodes)

	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].Source != "Alice" {
		t.Errorf("expected source Alice, got %v", links[0].Source)
	}
	if links[0].Target != "Bob" {
		t.Errorf("expected target Bob, got %v", links[0].Target)
	}
}

func TestConvertEdgesOrphanSkipped(t *testing.T) {
	nodes := testNodes()
	edges := []*graph.Edge{
		{SourceID: 1, TargetID: 999, Type: "MISSING"},
		{SourceID: 1, TargetID: 2, Type: "KNOWS"},
	}
	links := viz.ConvertEdges(edges, nodes)

	if len(links) != 1 {
		t.Fatalf("expected 1 link (orphan skipped), got %d", len(links))
	}
}

func TestNodeNameIndex(t *testing.T) {
	nodes := testNodes()
	idx := viz.NodeNameIndex(nodes)

	if idx[1] != "Alice" {
		t.Errorf("expected Alice for ID 1, got %s", idx[1])
	}
	if idx[3] != "Acme" {
		t.Errorf("expected Acme for ID 3, got %s", idx[3])
	}
}

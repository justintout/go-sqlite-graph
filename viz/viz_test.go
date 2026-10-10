package viz_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	graph "github.com/justintout/go-sqlite-graph"
	"github.com/justintout/go-sqlite-graph/viz"
)

func TestNewDefaults(t *testing.T) {
	c := viz.New(testNodes(), testEdges())

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	html := buf.String()
	if len(html) == 0 {
		t.Fatal("expected non-empty HTML output")
	}
}

func TestWithTitle(t *testing.T) {
	c := viz.New(testNodes(), testEdges(), viz.WithTitle("Test Graph"))

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	if !strings.Contains(buf.String(), "Test Graph") {
		t.Error("expected HTML to contain title 'Test Graph'")
	}
}

func TestWithLayout(t *testing.T) {
	c := viz.New(testNodes(), testEdges(), viz.WithLayout(viz.CircularLayout))

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	if !strings.Contains(buf.String(), "circular") {
		t.Error("expected HTML to contain 'circular' layout")
	}
}

func TestWithSize(t *testing.T) {
	c := viz.New(testNodes(), testEdges(), viz.WithSize("1200px", "800px"))

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "1200px") {
		t.Error("expected HTML to contain width '1200px'")
	}
}

func TestWithPalette(t *testing.T) {
	colors := []string{"#FF0000", "#00FF00"}
	c := viz.New(testNodes(), testEdges(), viz.WithPalette(colors))

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "#FF0000") {
		t.Error("expected HTML to contain custom color '#FF0000'")
	}
}

func TestRenderContainsNodeNames(t *testing.T) {
	c := viz.New(testNodes(), testEdges())

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	html := buf.String()
	for _, name := range []string{"Alice", "Bob", "Acme"} {
		if !strings.Contains(html, name) {
			t.Errorf("expected HTML to contain node name %q", name)
		}
	}
}

func TestFromQuery(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open("file:TestFromQuery?mode=memory&cache=shared", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	alice := &graph.Node{Name: "Alice", Labels: []string{"Person"}}
	bob := &graph.Node{Name: "Bob", Labels: []string{"Person"}}
	acme := &graph.Node{Name: "Acme", Labels: []string{"Company"}}
	for _, n := range []*graph.Node{alice, bob, acme} {
		if err := g.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []*graph.Edge{
		{SourceID: alice.ID, TargetID: bob.ID, Type: "KNOWS"},
		{SourceID: bob.ID, TargetID: acme.ID, Type: "WORKS_AT"},
	} {
		if err := g.CreateEdge(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	// From Acme, WORKS_AT in both directions reaches Bob, then Acme again.
	// The result excludes Alice, so the KNOWS edge must not be drawn.
	q := g.Match("Company").RelatedDir("WORKS_AT", graph.Both, 1, 2).Return("name")
	c, err := viz.FromQuery(ctx, g, q, viz.WithTitle("From Query"))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	html := buf.String()
	for _, want := range []string{"From Query", `"name":"Company"`, `"name":"Person"`, "WORKS_AT"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected HTML to contain %s", want)
		}
	}
	if strings.Contains(html, "KNOWS") {
		t.Error("rendered an edge to a node outside the result")
	}
}

func TestEmptyGraph(t *testing.T) {
	c := viz.New(nil, nil)

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	if len(buf.String()) == 0 {
		t.Fatal("expected non-empty HTML even with no data")
	}
}

func TestRenderContainsEdgeTypes(t *testing.T) {
	c := viz.New(testNodes(), testEdges())

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	html := buf.String()
	for _, edgeType := range []string{"KNOWS", "WORKS_AT"} {
		if !strings.Contains(html, edgeType) {
			t.Errorf("expected HTML to contain edge type %q", edgeType)
		}
	}
}

func TestRenderDuplicateNames(t *testing.T) {
	nodes := []*graph.Node{{ID: 1, Name: "Sam"}, {ID: 2, Name: "Sam"}}
	edges := []*graph.Edge{{SourceID: 1, TargetID: 2, Type: "KNOWS"}}

	var buf bytes.Buffer
	if err := viz.New(nodes, edges).Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	html := buf.String()
	for _, want := range []string{`"id":"1"`, `"id":"2"`, `"source":"1"`, `"target":"2"`} {
		if !strings.Contains(html, want) {
			t.Errorf("expected HTML to contain %s", want)
		}
	}
}

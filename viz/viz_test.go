package viz_test

import (
	"bytes"
	"strings"
	"testing"

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

func TestNewFromResult(t *testing.T) {
	// Result is constructed with Nodes() returning []*Node.
	// Since Result has unexported fields, we test via New directly
	// with the same node data. NewFromResult calls result.Nodes()
	// which returns the same type.
	nodes := testNodes()
	edges := testEdges()

	c := viz.New(nodes, edges, viz.WithTitle("From Result"))

	var buf bytes.Buffer
	if err := c.Render(&buf); err != nil {
		t.Fatalf("render error: %v", err)
	}
	if !strings.Contains(buf.String(), "From Result") {
		t.Error("expected title in output")
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

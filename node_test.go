package graph

import (
	"context"
	"testing"
)

func TestNodeCRUD(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()

	// Create
	n := &Node{
		Name:       "Alice",
		Labels:     []string{"Person", "Employee"},
		Properties: map[string]any{"age": 30, "city": "NYC"},
	}
	if err := g.CreateNode(ctx, n); err != nil {
		t.Fatal(err)
	}
	if n.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
	if n.CreatedAt == "" {
		t.Error("expected created_at to be set")
	}

	// Get
	got, err := g.GetNode(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Alice" {
		t.Errorf("name = %q, want %q", got.Name, "Alice")
	}
	if len(got.Labels) != 2 {
		t.Errorf("labels = %v, want 2 labels", got.Labels)
	}
	if got.Properties["city"] != "NYC" {
		t.Errorf("properties[city] = %v, want NYC", got.Properties["city"])
	}

	// Update
	n.Name = "Alice Smith"
	n.Properties["age"] = 31
	if err := g.UpdateNode(ctx, n); err != nil {
		t.Fatal(err)
	}
	got, err = g.GetNode(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Alice Smith" {
		t.Errorf("name = %q, want %q", got.Name, "Alice Smith")
	}

	// Delete
	if err := g.DeleteNode(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	_, err = g.GetNode(ctx, n.ID)
	if err == nil {
		t.Error("expected error getting deleted node")
	}
}

func TestNodeLabels(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()

	n := &Node{Name: "Bob", Labels: []string{"Person"}}
	if err := g.CreateNode(ctx, n); err != nil {
		t.Fatal(err)
	}

	// Add labels
	if err := g.AddLabels(ctx, n.ID, "Developer", "Person"); err != nil { // Person is duplicate, should be ignored
		t.Fatal(err)
	}
	got, _ := g.GetNode(ctx, n.ID)
	if len(got.Labels) != 2 {
		t.Errorf("labels = %v, want [Developer Person]", got.Labels)
	}

	// Remove labels
	if err := g.RemoveLabels(ctx, n.ID, "Developer"); err != nil {
		t.Fatal(err)
	}
	got, _ = g.GetNode(ctx, n.ID)
	if len(got.Labels) != 1 || got.Labels[0] != "Person" {
		t.Errorf("labels = %v, want [Person]", got.Labels)
	}
}

func TestDeleteNodeCascade(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()

	a := &Node{Name: "A", Labels: []string{"X"}}
	b := &Node{Name: "B", Labels: []string{"X"}}
	g.CreateNode(ctx, a)
	g.CreateNode(ctx, b)

	e := &Edge{SourceID: a.ID, TargetID: b.ID, Type: "LINKS"}
	g.CreateEdge(ctx, e)

	// Deleting A should cascade to the edge
	if err := g.DeleteNode(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	_, err := g.GetEdge(ctx, e.ID)
	if err == nil {
		t.Error("expected edge to be deleted via cascade")
	}
}

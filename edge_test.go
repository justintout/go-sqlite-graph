package graph

import (
	"context"
	"testing"
)

func TestEdgeCRUD(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()

	a := &Node{Name: "Alice", Labels: []string{"Person"}}
	b := &Node{Name: "Bob", Labels: []string{"Person"}}
	g.CreateNode(ctx, a)
	g.CreateNode(ctx, b)

	// Create
	e := &Edge{
		SourceID:   a.ID,
		TargetID:   b.ID,
		Type:       "KNOWS",
		Name:       "friendship",
		Properties: map[string]any{"since": 2020},
	}
	if err := g.CreateEdge(ctx, e); err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
	if e.CreatedAt == "" {
		t.Error("expected created_at to be set")
	}

	// Get
	got, err := g.GetEdge(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "KNOWS" {
		t.Errorf("type = %q, want KNOWS", got.Type)
	}
	if got.SourceID != a.ID || got.TargetID != b.ID {
		t.Error("source/target IDs mismatch")
	}

	// Update
	e.Name = "close friendship"
	e.Properties["strength"] = 10
	if err := g.UpdateEdge(ctx, e); err != nil {
		t.Fatal(err)
	}
	got, _ = g.GetEdge(ctx, e.ID)
	if got.Name != "close friendship" {
		t.Errorf("name = %q, want %q", got.Name, "close friendship")
	}

	// Delete
	if err := g.DeleteEdge(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	_, err = g.GetEdge(ctx, e.ID)
	if err == nil {
		t.Error("expected error getting deleted edge")
	}
}

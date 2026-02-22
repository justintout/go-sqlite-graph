package graph

import (
	"context"
	"testing"
)

func TestTxCommit(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()

	tx, err := g.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}

	n := &Node{Name: "TxNode", Labels: []string{"Test"}}
	if err := tx.CreateNode(ctx, n); err != nil {
		t.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Node should be visible outside the transaction
	got, err := g.GetNode(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "TxNode" {
		t.Errorf("name = %q, want TxNode", got.Name)
	}
}

func TestTxRollback(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()

	tx, err := g.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}

	n := &Node{Name: "WillRollback", Labels: []string{"Test"}}
	if err := tx.CreateNode(ctx, n); err != nil {
		t.Fatal(err)
	}
	nodeID := n.ID

	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	// Node should NOT be visible
	_, err = g.GetNode(ctx, nodeID)
	if err == nil {
		t.Error("expected error: node should not exist after rollback")
	}
}

func TestTxDeferPattern(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()

	tx, err := g.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	n := &Node{Name: "DeferNode", Labels: []string{"Test"}}
	if err := tx.CreateNode(ctx, n); err != nil {
		t.Fatal(err)
	}

	e := &Edge{SourceID: n.ID, TargetID: n.ID, Type: "SELF"}
	if err := tx.CreateEdge(ctx, e); err != nil {
		t.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Both should exist
	if _, err := g.GetNode(ctx, n.ID); err != nil {
		t.Errorf("node not found: %v", err)
	}
	if _, err := g.GetEdge(ctx, e.ID); err != nil {
		t.Errorf("edge not found: %v", err)
	}
}

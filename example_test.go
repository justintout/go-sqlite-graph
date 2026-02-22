package graph_test

import (
	"context"
	"fmt"
	"log"

	graph "github.com/justintout/go-sqlite-graph"
)

func Example() {
	ctx := context.Background()

	// Open an in-memory graph database (auto-migrates schema).
	g, err := graph.Open("file:example?mode=memory&cache=shared", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer g.Close()

	// Create nodes with labels and properties.
	alice := &graph.Node{
		Name:       "Alice",
		Labels:     []string{"Person", "Engineer"},
		Properties: map[string]any{"age": 30, "city": "NYC"},
	}
	bob := &graph.Node{
		Name:       "Bob",
		Labels:     []string{"Person"},
		Properties: map[string]any{"age": 25, "city": "SF"},
	}
	charlie := &graph.Node{
		Name:       "Charlie",
		Labels:     []string{"Person"},
		Properties: map[string]any{"age": 35, "city": "NYC"},
	}
	acme := &graph.Node{
		Name:       "Acme Corp",
		Labels:     []string{"Company"},
		Properties: map[string]any{"industry": "tech"},
	}

	for _, n := range []*graph.Node{alice, bob, charlie, acme} {
		if err := g.CreateNode(ctx, n); err != nil {
			log.Fatal(err)
		}
	}

	// Create edges (relationships) between nodes.
	edges := []*graph.Edge{
		{SourceID: alice.ID, TargetID: bob.ID, Type: "KNOWS", Properties: map[string]any{"since": 2020}},
		{SourceID: bob.ID, TargetID: charlie.ID, Type: "KNOWS", Properties: map[string]any{"since": 2022}},
		{SourceID: alice.ID, TargetID: acme.ID, Type: "WORKS_AT"},
		{SourceID: bob.ID, TargetID: acme.ID, Type: "WORKS_AT"},
	}
	for _, e := range edges {
		if err := g.CreateEdge(ctx, e); err != nil {
			log.Fatal(err)
		}
	}

	// Query: find all Person nodes.
	res, err := g.Match("Person").Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("All people (%d):\n", res.Len())
	for _, n := range res.Nodes() {
		fmt.Printf("  %s\n", n.Name)
	}

	// Query: find people older than 28.
	res, err = g.Match("Person").WhereJSON("age", ">", 28).Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("People older than 28 (%d):\n", res.Len())
	for _, n := range res.Nodes() {
		fmt.Printf("  %s\n", n.Name)
	}

	// Query: who does Alice know directly (1 hop)?
	res, err = g.Match("Person").
		Where("name", "=", "Alice").
		Related("KNOWS", 1, 1).
		Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Alice knows directly:\n")
	for _, n := range res.Nodes() {
		fmt.Printf("  %s\n", n.Name)
	}

	// Query: who does Alice know within 2 hops?
	res, err = g.Match("Person").
		Where("name", "=", "Alice").
		Related("KNOWS", 1, 2).
		Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Alice knows within 2 hops:\n")
	for _, n := range res.Nodes() {
		fmt.Printf("  %s\n", n.Name)
	}

	// Query: where do Alice's friends work?
	res, err = g.Match("Person").
		Where("name", "=", "Alice").
		Related("KNOWS", 1, 1).
		Related("WORKS_AT", 1, 1).
		Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Alice's friends work at:\n")
	for _, n := range res.Nodes() {
		fmt.Printf("  %s\n", n.Name)
	}

	// Transactions: batch operations atomically.
	tx, err := g.BeginTx(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	diana := &graph.Node{
		Name:       "Diana",
		Labels:     []string{"Person"},
		Properties: map[string]any{"age": 28},
	}
	if err := tx.CreateNode(ctx, diana); err != nil {
		log.Fatal(err)
	}
	if err := tx.CreateEdge(ctx, &graph.Edge{
		SourceID: charlie.ID, TargetID: diana.ID, Type: "KNOWS",
	}); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	// Count people after the transaction.
	count, err := g.Match("Person").Count(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Total people: %d\n", count)

	// Output:
	// All people (3):
	//   Alice
	//   Bob
	//   Charlie
	// People older than 28 (2):
	//   Alice
	//   Charlie
	// Alice knows directly:
	//   Bob
	// Alice knows within 2 hops:
	//   Bob
	//   Charlie
	// Alice's friends work at:
	//   Acme Corp
	// Total people: 4
}

package graph

import (
	"context"
	"testing"
)

// buildSocialGraph creates a test graph:
//
//	Alice -KNOWS-> Bob -KNOWS-> Charlie -KNOWS-> Diana
//	                            Charlie -KNOWS-> Eve
//	Alice -WORKS_AT-> Acme
//	Bob   -WORKS_AT-> Globex
//
// All people have age properties. Companies have industry properties.
func buildSocialGraph(t *testing.T, g *Graph) (alice, bob, charlie, diana, eve, acme, globex *Node) {
	t.Helper()
	ctx := context.Background()

	alice = &Node{Name: "Alice", Labels: []string{"Person"}, Properties: map[string]any{"age": 30}}
	bob = &Node{Name: "Bob", Labels: []string{"Person"}, Properties: map[string]any{"age": 25}}
	charlie = &Node{Name: "Charlie", Labels: []string{"Person"}, Properties: map[string]any{"age": 35}}
	diana = &Node{Name: "Diana", Labels: []string{"Person"}, Properties: map[string]any{"age": 28}}
	eve = &Node{Name: "Eve", Labels: []string{"Person"}, Properties: map[string]any{"age": 40}}
	acme = &Node{Name: "Acme", Labels: []string{"Company"}, Properties: map[string]any{"industry": "tech"}}
	globex = &Node{Name: "Globex", Labels: []string{"Company"}, Properties: map[string]any{"industry": "finance"}}

	for _, n := range []*Node{alice, bob, charlie, diana, eve, acme, globex} {
		if err := g.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	edges := []*Edge{
		{SourceID: alice.ID, TargetID: bob.ID, Type: "KNOWS"},
		{SourceID: bob.ID, TargetID: charlie.ID, Type: "KNOWS"},
		{SourceID: charlie.ID, TargetID: diana.ID, Type: "KNOWS"},
		{SourceID: charlie.ID, TargetID: eve.ID, Type: "KNOWS"},
		{SourceID: alice.ID, TargetID: acme.ID, Type: "WORKS_AT"},
		{SourceID: bob.ID, TargetID: globex.ID, Type: "WORKS_AT"},
	}
	for _, e := range edges {
		if err := g.CreateEdge(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	return
}

func TestMatchByLabel(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	res, err := g.Match("Person").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Len() != 5 {
		t.Errorf("got %d persons, want 5", res.Len())
	}

	res, err = g.Match("Company").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Len() != 2 {
		t.Errorf("got %d companies, want 2", res.Len())
	}
}

func TestMatchWhere(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	res, err := g.Match("Person").Where("name", "=", "Alice").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Len() != 1 {
		t.Fatalf("got %d results, want 1", res.Len())
	}
	if res.Nodes()[0].Name != "Alice" {
		t.Errorf("name = %q, want Alice", res.Nodes()[0].Name)
	}
}

func TestMatchWhereJSON(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	res, err := g.Match("Person").WhereJSON("age", ">", 30).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	names := nodeNames(res.Nodes())
	// Charlie (35) and Eve (40)
	if len(names) != 2 {
		t.Errorf("got %v, want [Charlie Eve]", names)
	}
	if !containsAll(names, "Charlie", "Eve") {
		t.Errorf("got %v, expected Charlie and Eve", names)
	}
}

func TestSingleHopRelated(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	// Alice's direct friends
	res, err := g.Match("Person").
		Where("name", "=", "Alice").
		Related("KNOWS", 1, 1).
		Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := nodeNames(res.Nodes())
	if len(names) != 1 || names[0] != "Bob" {
		t.Errorf("got %v, want [Bob]", names)
	}
}

func TestMultiHopRelated(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	// Alice -> KNOWS -> 1 to 3 hops
	res, err := g.Match("Person").
		Where("name", "=", "Alice").
		Related("KNOWS", 1, 3).
		Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := nodeNames(res.Nodes())
	// Bob (1 hop), Charlie (2 hops), Diana (3 hops), Eve (3 hops)
	if !containsAll(names, "Bob", "Charlie", "Diana", "Eve") {
		t.Errorf("got %v, want [Bob Charlie Diana Eve]", names)
	}
	if len(names) != 4 {
		t.Errorf("got %d results, want 4", len(names))
	}
}

func TestMultiHopMinHops(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	// Only 2+ hops from Alice via KNOWS
	res, err := g.Match("Person").
		Where("name", "=", "Alice").
		Related("KNOWS", 2, 3).
		Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := nodeNames(res.Nodes())
	// Charlie (2 hops), Diana (3 hops), Eve (3 hops) — NOT Bob (1 hop)
	if containsAny(names, "Bob") {
		t.Errorf("got %v, should not include Bob (only 1 hop)", names)
	}
	if !containsAll(names, "Charlie", "Diana", "Eve") {
		t.Errorf("got %v, want [Charlie Diana Eve]", names)
	}
}

func TestChainedRelated(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	// Alice -> KNOWS -> 1 hop -> WORKS_AT -> 1 hop
	// Alice knows Bob, Bob works at Globex
	res, err := g.Match("Person").
		Where("name", "=", "Alice").
		Related("KNOWS", 1, 1).
		Related("WORKS_AT", 1, 1).
		Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := nodeNames(res.Nodes())
	if len(names) != 1 || names[0] != "Globex" {
		t.Errorf("got %v, want [Globex]", names)
	}
}

func TestWhereRelFilter(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	// Alice -> KNOWS (1-2 hops) -> WORKS_AT -> company named "Globex"
	res, err := g.Match("Person").
		Where("name", "=", "Alice").
		Related("KNOWS", 1, 2).
		Related("WORKS_AT", 1, 1).
		WhereRel("name", "=", "Globex").
		Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := nodeNames(res.Nodes())
	if len(names) != 1 || names[0] != "Globex" {
		t.Errorf("got %v, want [Globex]", names)
	}
}

func TestCycleHandling(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()

	// Create a cycle: A -> B -> C -> A
	a := &Node{Name: "A", Labels: []string{"Cyclic"}}
	b := &Node{Name: "B", Labels: []string{"Cyclic"}}
	c := &Node{Name: "C", Labels: []string{"Cyclic"}}
	g.CreateNode(ctx, a)
	g.CreateNode(ctx, b)
	g.CreateNode(ctx, c)

	g.CreateEdge(ctx, &Edge{SourceID: a.ID, TargetID: b.ID, Type: "NEXT"})
	g.CreateEdge(ctx, &Edge{SourceID: b.ID, TargetID: c.ID, Type: "NEXT"})
	g.CreateEdge(ctx, &Edge{SourceID: c.ID, TargetID: a.ID, Type: "NEXT"})

	// Traverse 1-5 hops — should not infinite loop
	res, err := g.Match("Cyclic").
		Where("name", "=", "A").
		Related("NEXT", 1, 5).
		Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Should get B and C (and possibly A again at depth 3), but no hang
	if res.Len() == 0 {
		t.Error("expected some results from cycle traversal")
	}
	t.Logf("cycle traversal returned %d results", res.Len())
}

func TestCount(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	count, err := g.Match("Person").Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

func TestLimit(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	res, err := g.Match("Person").Limit(2).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Len() != 2 {
		t.Errorf("got %d results, want 2", res.Len())
	}
}

func TestIncomingDirection(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	// Who KNOWS Bob? (incoming) — should be Alice
	res, err := g.Match("Person").
		Where("name", "=", "Bob").
		RelatedDir("KNOWS", Incoming, 1, 1).
		Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := nodeNames(res.Nodes())
	if len(names) != 1 || names[0] != "Alice" {
		t.Errorf("got %v, want [Alice]", names)
	}
}

func TestQueryInTransaction(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	tx, err := g.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	res, err := tx.Match("Person").Where("name", "=", "Alice").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Len() != 1 {
		t.Errorf("got %d results in tx, want 1", res.Len())
	}

	tx.Commit()
}

// helpers

func nodeNames(nodes []*Node) []string {
	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name
	}
	return names
}

func containsAll(haystack []string, needles ...string) bool {
	set := make(map[string]bool)
	for _, s := range haystack {
		set[s] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}

func containsAny(haystack []string, needles ...string) bool {
	set := make(map[string]bool)
	for _, s := range haystack {
		set[s] = true
	}
	for _, n := range needles {
		if set[n] {
			return true
		}
	}
	return false
}

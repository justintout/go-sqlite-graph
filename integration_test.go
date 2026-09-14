package graph

import (
	"context"
	"fmt"
	"maps"
	"math/rand"
	"slices"
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

func TestWhereIn(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	cases := []struct {
		name string
		q    *Query
		want []string
	}{
		{"column", g.Match("Person").Where("name", "in", []string{"Alice", "Eve", "Nobody"}), []string{"Alice", "Eve"}},
		{"json ints", g.Match("Person").WhereJSON("age", "IN", []int{25, 40}), []string{"Bob", "Eve"}},
		{"not in", g.Match("Person").WhereJSON("age", "NOT IN", []any{25, 30, 35}), []string{"Diana", "Eve"}},
		{"rel", g.Match("Person").Where("name", "=", "Alice").Related("KNOWS", 1, 3).WhereRel("name", "IN", []string{"Bob", "Diana"}), []string{"Bob", "Diana"}},
		{"empty", g.Match("Person").Where("name", "IN", []string{}), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := c.q.Run(ctx)
			if err != nil {
				t.Fatal(err)
			}
			names := nodeNames(res.Nodes())
			if len(names) != len(c.want) || !containsAll(names, c.want...) {
				t.Errorf("got %v, want %v", names, c.want)
			}
		})
	}

	if _, err := g.Match("Person").Where("name", "IN", "Alice").Run(ctx); err == nil {
		t.Error("IN accepted a non-slice value")
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

func TestWhereRelIntermediateStep(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	// Alice knows Bob and Charlie within 2 hops. Filtering to Bob reaches
	// Globex; filtering to Charlie, who works nowhere, reaches nothing.
	for _, bfs := range []bool{false, true} {
		for filter, want := range map[string]int{"Bob": 1, "Charlie": 0} {
			q := g.Match("Person").Where("name", "=", "Alice").Related("KNOWS", 1, 2)
			if bfs {
				q = q.BreadthFirst()
			}
			res, err := q.WhereRel("name", "=", filter).Related("WORKS_AT", 1, 1).Run(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if res.Len() != want {
				t.Errorf("breadthFirst=%v filter=%s: got %v, want %d results", bfs, filter, nodeNames(res.Nodes()), want)
			}
		}
	}
}

func TestBothDirection(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	for _, maxHops := range []int{1, 2} {
		res, err := g.Match("Person").
			Where("name", "=", "Charlie").
			RelatedDir("KNOWS", Both, 1, maxHops).
			Run(ctx)
		if err != nil {
			t.Fatal(err)
		}
		names := nodeNames(res.Nodes())
		want := []string{"Bob", "Diana", "Eve"}
		if maxHops == 2 {
			// Alice via Bob; Charlie via any neighbor and back.
			want = append(want, "Alice", "Charlie")
		}
		if len(names) != len(want) || !containsAll(names, want...) {
			t.Errorf("maxHops=%d: got %v, want %v", maxHops, names, want)
		}
	}
}

func TestFilterInjection(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	buildSocialGraph(t, g)

	evil := "name = name OR 1=1; DROP TABLE nodes; --"
	if _, err := g.Match("Person").Where(evil, "=", "x").Run(ctx); err == nil {
		t.Error("Where accepted an arbitrary column name")
	}
	if _, err := g.Match("Person").Related("KNOWS", 1, 1).WhereRel(evil, "=", "x").Run(ctx); err == nil {
		t.Error("WhereRel accepted an arbitrary column name")
	}

	// A hostile path is bound as a JSON path: it matches nothing and leaves the data intact.
	path := "age' IS NOT NULL OR 1=1 OR '"
	for _, q := range []*Query{
		g.Match("Person").WhereJSON(path, "=", 30),
		g.Match("Person").Related("KNOWS", 1, 1).WhereRelJSON(path, "=", 30),
		g.Match("Person").Return(path),
	} {
		res, err := q.Run(ctx)
		if err == nil && q.returnCols == nil && res.Len() != 0 {
			t.Errorf("hostile path matched %v", nodeNames(res.Nodes()))
		}
	}
	if n, err := g.Match("Person").Count(ctx); err != nil || n != 5 {
		t.Errorf("count = %d, %v; want 5", n, err)
	}
}

func TestReturn(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	alice, _, _, _, _, _, _ := buildSocialGraph(t, g)

	// Traversal from Alice via WORKS_AT reaches Acme, which has industry but no age.
	for _, q := range []*Query{
		g.Match("Person").Where("name", "=", "Alice").Return("name", "age", "missing"),
		g.Match("Person").Where("name", "=", "Alice").Related("KNOWS", 1, 1).RelatedDir("KNOWS", Incoming, 1, 1).Return("id", "name", "age"),
	} {
		res, err := q.Run(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.Len() != 1 {
			t.Fatalf("got %d results, want 1", res.Len())
		}
		n := res.Nodes()[0]
		if n.ID != alice.ID || n.Name != "Alice" || n.CreatedAt != "" {
			t.Errorf("got %+v, want ID %d, name Alice, no created_at", n, alice.ID)
		}
		if len(n.Properties) != 1 || n.Properties["age"] != float64(30) {
			t.Errorf("properties = %v, want map[age:30]", n.Properties)
		}
	}

	if err := g.AddLabels(ctx, alice.ID, "Engineer"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []*Query{
		g.Match("Person").Where("name", "=", "Alice").Return("labels"),
		g.Match("Company").RelatedDir("WORKS_AT", Incoming, 1, 1).WhereRel("name", "=", "Alice").Return("labels"),
	} {
		res, err := q.Run(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got := res.Nodes()[0].Labels; !slices.Equal(got, []string{"Engineer", "Person"}) {
			t.Errorf("labels = %v, want [Engineer Person]", got)
		}
	}

	res, err := g.Match("Company").Return("properties", "industry").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Nodes() {
		if n.Name != "" || n.Properties["industry"] == nil {
			t.Errorf("got %+v, want only properties", n)
		}
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

	res, err = g.Match("Person").Offset(3).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Len() != 2 {
		t.Errorf("offset only: got %d results, want 2", res.Len())
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

// TestTraversalMatchesReference checks every traversal compilation path
// against walks computed in Go on a random graph with cycles and self-loops.
func TestTraversalMatchesReference(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()
	rng := rand.New(rand.NewSource(7))

	const n = 60
	ids := make([]int64, n)
	index := map[int64]int{}
	for i := range n {
		node := &Node{Name: fmt.Sprintf("v%d", i), Labels: []string{"V"}}
		if err := g.CreateNode(ctx, node); err != nil {
			t.Fatal(err)
		}
		ids[i] = node.ID
		index[node.ID] = i
	}
	out := make([][]int, n)
	in := make([][]int, n)
	for range 2 * n {
		s, d := rng.Intn(n), rng.Intn(n)
		if err := g.CreateEdge(ctx, &Edge{SourceID: ids[s], TargetID: ids[d], Type: "E"}); err != nil {
			t.Fatal(err)
		}
		out[s] = append(out[s], d)
		in[d] = append(in[d], s)
	}

	reference := func(start int, dir Direction, minHops, maxHops int) map[string]bool {
		frontier := map[int]bool{start: true}
		reached := map[string]bool{}
		for depth := 1; depth <= maxHops; depth++ {
			next := map[int]bool{}
			for v := range frontier {
				if dir != Incoming {
					for _, u := range out[v] {
						next[u] = true
					}
				}
				if dir != Outgoing {
					for _, u := range in[v] {
						next[u] = true
					}
				}
			}
			if depth >= minHops {
				for v := range next {
					reached[fmt.Sprintf("v%d", v)] = true
				}
			}
			frontier = next
		}
		return reached
	}

	for _, dir := range []Direction{Outgoing, Incoming, Both} {
		for _, minHops := range []int{1, 2} {
			for maxHops := minHops; maxHops <= 6; maxHops++ {
				for _, start := range []int{0, 17, 42} {
					for _, bfs := range []bool{false, true} {
						if bfs && minHops != 1 {
							continue
						}
						q := g.Match("V").
							Where("name", "=", fmt.Sprintf("v%d", start)).
							RelatedDir("E", dir, minHops, maxHops)
						if bfs {
							q = q.BreadthFirst()
						}
						res, err := q.Return("name").Run(ctx)
						if err != nil {
							t.Fatal(err)
						}
						want := reference(start, dir, minHops, maxHops)
						got := map[string]bool{}
						for _, node := range res.Nodes() {
							got[node.Name] = true
						}
						if res.Len() != len(got) || !maps.Equal(got, want) {
							t.Errorf("dir=%d hops=%d..%d start=v%d breadthFirst=%v: got %d nodes (%d rows), want %d",
								dir, minHops, maxHops, start, bfs, len(got), res.Len(), len(want))
						}
					}
				}
			}
		}
	}
}

func TestBreadthFirstRequiresMinHopsOne(t *testing.T) {
	g := openTestGraph(t)
	if _, err := g.Match("V").Related("E", 2, 4).BreadthFirst().Run(context.Background()); err == nil {
		t.Error("BreadthFirst accepted minHops of 2")
	}
}

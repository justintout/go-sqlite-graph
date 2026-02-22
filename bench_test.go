package graph

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
)

// openBenchGraph opens a shared-cache in-memory graph for benchmarks.
func openBenchGraph(b *testing.B) *Graph {
	b.Helper()
	uri := fmt.Sprintf("file:bench_%d?mode=memory&cache=shared", rand.Int())
	g, err := Open(uri, &Options{PoolSize: 2})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { g.Close() })
	return g
}

// buildChainGraph creates a linear chain of n nodes connected by LINK edges:
// n0 -LINK-> n1 -LINK-> n2 -LINK-> ... -LINK-> n(n-1)
// All nodes have the label "Node" and a JSON property "idx".
// Returns the first node's ID.
func buildChainGraph(b *testing.B, g *Graph, n int) int64 {
	b.Helper()
	ctx := context.Background()

	nodes := make([]*Node, n)
	for i := range n {
		nodes[i] = &Node{
			Name:       fmt.Sprintf("n%d", i),
			Labels:     []string{"Node"},
			Properties: map[string]any{"idx": i},
		}
		if err := g.CreateNode(ctx, nodes[i]); err != nil {
			b.Fatal(err)
		}
	}
	for i := range n - 1 {
		if err := g.CreateEdge(ctx, &Edge{
			SourceID: nodes[i].ID,
			TargetID: nodes[i+1].ID,
			Type:     "LINK",
		}); err != nil {
			b.Fatal(err)
		}
	}
	return nodes[0].ID
}

// buildFanoutGraph creates a tree where each node has `fanout` children, up to
// `depth` levels deep. All nodes have label "Node". The root has label "Root" too.
// Returns the root node's ID and total node count.
func buildFanoutGraph(b *testing.B, g *Graph, depth, fanout int) (rootID int64, totalNodes int) {
	b.Helper()
	ctx := context.Background()

	root := &Node{Name: "root", Labels: []string{"Node", "Root"}, Properties: map[string]any{"depth": 0}}
	if err := g.CreateNode(ctx, root); err != nil {
		b.Fatal(err)
	}

	totalNodes = 1
	currentLevel := []*Node{root}

	for d := 1; d <= depth; d++ {
		var nextLevel []*Node
		for _, parent := range currentLevel {
			for f := range fanout {
				child := &Node{
					Name:       fmt.Sprintf("d%d_f%d_%d", d, f, totalNodes),
					Labels:     []string{"Node"},
					Properties: map[string]any{"depth": d},
				}
				if err := g.CreateNode(ctx, child); err != nil {
					b.Fatal(err)
				}
				if err := g.CreateEdge(ctx, &Edge{
					SourceID: parent.ID,
					TargetID: child.ID,
					Type:     "CHILD",
				}); err != nil {
					b.Fatal(err)
				}
				totalNodes++
				nextLevel = append(nextLevel, child)
			}
		}
		currentLevel = nextLevel
	}
	return root.ID, totalNodes
}

// buildDenseGraph creates n nodes all labeled "Node", with random directed edges.
// Each node gets `edgesPerNode` random outgoing LINK edges.
// Returns a slice of all node IDs.
func buildDenseGraph(b *testing.B, g *Graph, n, edgesPerNode int) []int64 {
	b.Helper()
	ctx := context.Background()
	rng := rand.New(rand.NewSource(42))

	ids := make([]int64, n)
	for i := range n {
		node := &Node{
			Name:       fmt.Sprintf("n%d", i),
			Labels:     []string{"Node"},
			Properties: map[string]any{"idx": i},
		}
		if err := g.CreateNode(ctx, node); err != nil {
			b.Fatal(err)
		}
		ids[i] = node.ID
	}

	for i := range n {
		targets := rng.Perm(n)
		added := 0
		for _, t := range targets {
			if t == i {
				continue
			}
			if err := g.CreateEdge(ctx, &Edge{
				SourceID: ids[i],
				TargetID: ids[t],
				Type:     "LINK",
			}); err != nil {
				b.Fatal(err)
			}
			added++
			if added >= edgesPerNode {
				break
			}
		}
	}
	return ids
}

// BenchmarkTraversalChain measures recursive CTE performance on a linear chain
// at varying hop depths. The chain has 100 nodes; we query from node 0.
func BenchmarkTraversalChain(b *testing.B) {
	g := openBenchGraph(b)
	buildChainGraph(b, g, 100)
	ctx := context.Background()

	for _, hops := range []int{1, 2, 3, 5, 10} {
		b.Run(fmt.Sprintf("hops=%d", hops), func(b *testing.B) {
			b.ResetTimer()
			for range b.N {
				res, err := g.Match("Node").
					Where("name", "=", "n0").
					Related("LINK", 1, hops).
					Run(ctx)
				if err != nil {
					b.Fatal(err)
				}
				if res.Len() == 0 {
					b.Fatal("expected results")
				}
			}
		})
	}
}

// BenchmarkTraversalFanout measures recursive CTE performance on a tree
// with branching factor 3 at varying depths. Total nodes grow as 3^depth.
func BenchmarkTraversalFanout(b *testing.B) {
	for _, depth := range []int{2, 4, 6} {
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			g := openBenchGraph(b)
			_, totalNodes := buildFanoutGraph(b, g, depth, 3)
			b.Logf("tree: depth=%d fanout=3 nodes=%d", depth, totalNodes)
			ctx := context.Background()

			b.ResetTimer()
			for range b.N {
				res, err := g.Match("Root").
					Related("CHILD", 1, depth).
					Run(ctx)
				if err != nil {
					b.Fatal(err)
				}
				if res.Len() == 0 {
					b.Fatal("expected results")
				}
			}
		})
	}
}

// BenchmarkTraversalDense measures recursive CTE performance on a dense random
// graph (500 nodes, 5 edges per node) at varying hop depths.
func BenchmarkTraversalDense(b *testing.B) {
	g := openBenchGraph(b)
	ids := buildDenseGraph(b, g, 500, 5)
	ctx := context.Background()
	startName := fmt.Sprintf("n%d", 0)
	_ = ids

	for _, hops := range []int{1, 2, 3, 5, 10} {
		b.Run(fmt.Sprintf("hops=%d", hops), func(b *testing.B) {
			b.ResetTimer()
			for range b.N {
				res, err := g.Match("Node").
					Where("name", "=", startName).
					Related("LINK", 1, hops).
					Run(ctx)
				if err != nil {
					b.Fatal(err)
				}
				if res.Len() == 0 {
					b.Fatal("expected results")
				}
			}
		})
	}
}

// BenchmarkCreateNode measures single node creation throughput.
func BenchmarkCreateNode(b *testing.B) {
	g := openBenchGraph(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := range b.N {
		n := &Node{
			Name:       fmt.Sprintf("n%d", i),
			Labels:     []string{"Bench"},
			Properties: map[string]any{"i": i},
		}
		if err := g.CreateNode(ctx, n); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCreateEdge measures single edge creation throughput
// (between two pre-existing nodes).
func BenchmarkCreateEdge(b *testing.B) {
	g := openBenchGraph(b)
	ctx := context.Background()

	a := &Node{Name: "a", Labels: []string{"Bench"}}
	z := &Node{Name: "z", Labels: []string{"Bench"}}
	g.CreateNode(ctx, a)
	g.CreateNode(ctx, z)

	b.ResetTimer()
	for i := range b.N {
		e := &Edge{
			SourceID: a.ID,
			TargetID: z.ID,
			Type:     "BENCH",
			Name:     fmt.Sprintf("e%d", i),
		}
		if err := g.CreateEdge(ctx, e); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBulkInsertTx measures bulk node+edge creation inside a single transaction.
func BenchmarkBulkInsertTx(b *testing.B) {
	for _, batchSize := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("batch=%d", batchSize), func(b *testing.B) {
			g := openBenchGraph(b)
			ctx := context.Background()

			b.ResetTimer()
			for range b.N {
				tx, err := g.BeginTx(ctx)
				if err != nil {
					b.Fatal(err)
				}

				var prev *Node
				for i := range batchSize {
					n := &Node{
						Name:       fmt.Sprintf("n%d", i),
						Labels:     []string{"Batch"},
						Properties: map[string]any{"i": i},
					}
					if err := tx.CreateNode(ctx, n); err != nil {
						tx.Rollback()
						b.Fatal(err)
					}
					if prev != nil {
						if err := tx.CreateEdge(ctx, &Edge{
							SourceID: prev.ID,
							TargetID: n.ID,
							Type:     "SEQ",
						}); err != nil {
							tx.Rollback()
							b.Fatal(err)
						}
					}
					prev = n
				}

				if err := tx.Commit(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMatchSimple measures simple label match (no traversal) on a graph
// with varying node counts.
func BenchmarkMatchSimple(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("nodes=%d", n), func(b *testing.B) {
			g := openBenchGraph(b)
			ctx := context.Background()

			for i := range n {
				node := &Node{
					Name:   fmt.Sprintf("n%d", i),
					Labels: []string{"Item"},
				}
				if err := g.CreateNode(ctx, node); err != nil {
					b.Fatal(err)
				}
			}

			b.ResetTimer()
			for range b.N {
				res, err := g.Match("Item").
					Where("name", "=", "n0").
					Run(ctx)
				if err != nil {
					b.Fatal(err)
				}
				if res.Len() != 1 {
					b.Fatalf("expected 1 result, got %d", res.Len())
				}
			}
		})
	}
}

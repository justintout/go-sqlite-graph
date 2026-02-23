package viz_test

import (
	"fmt"
	"log"
	"os"

	graph "github.com/justintout/go-sqlite-graph"
	"github.com/justintout/go-sqlite-graph/viz"
)

func Example() {
	nodes := []*graph.Node{
		{ID: 1, Name: "Alice", Labels: []string{"Person"}},
		{ID: 2, Name: "Bob", Labels: []string{"Person"}},
		{ID: 3, Name: "Acme Corp", Labels: []string{"Company"}},
	}

	edges := []*graph.Edge{
		{SourceID: 1, TargetID: 2, Type: "KNOWS"},
		{SourceID: 1, TargetID: 3, Type: "WORKS_AT"},
	}

	c := viz.New(nodes, edges,
		viz.WithTitle("Social Graph"),
		viz.WithLayout(viz.ForceLayout),
	)

	f, err := os.CreateTemp("", "graph-*.html")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer f.Close()

	if err := c.Render(f); err != nil {
		fmt.Println("error:", err)
		return
	}
	log.Println("graph written to", f.Name())

	fi, _ := f.Stat()
	fmt.Printf("rendered %d bytes of HTML\n", fi.Size())
	// Output:
	// rendered 1616 bytes of HTML
}

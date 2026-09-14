// Package compare benchmarks go-sqlite-graph against GraphQLite and against
// go-sqlite-graph's own SQL run on C SQLite. It is a separate module so its
// CGo dependency stays out of the library.
package compare

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	graph "github.com/justintout/go-sqlite-graph"
	"github.com/mattn/go-sqlite3"
)

// graphSpec describes a generated graph. Every node has label Node and name
// "n<i>"; every edge has type LINK.
type graphSpec struct {
	name  string
	nodes int
	edges int
	// zipf draws edge targets from a Zipf distribution, giving a few hubs
	// with large in-degree. Otherwise each node gets edges/nodes uniform
	// random targets.
	zipf bool
}

var (
	random10k   = graphSpec{name: "random-10k", nodes: 10_000, edges: 30_000}
	powerLaw100 = graphSpec{name: "powerlaw-100k", nodes: 100_000, edges: 500_000, zipf: true}
)

func (s graphSpec) generate() [][2]int {
	rng := rand.New(rand.NewSource(42))
	out := make([][2]int, 0, s.edges)
	if s.zipf {
		// s=1.01, v=5 gives node 0 roughly 10k in-edges on 100k/500k.
		z := rand.NewZipf(rng, 1.01, 5, uint64(s.nodes-1))
		for range s.edges {
			out = append(out, [2]int{rng.Intn(s.nodes), int(z.Uint64())})
		}
		return out
	}
	per := s.edges / s.nodes
	for i := range s.nodes {
		for range per {
			out = append(out, [2]int{i, rng.Intn(s.nodes)})
		}
	}
	return out
}

type query struct {
	name    string
	start   int
	in      bool
	maxHops int
}

func (s graphSpec) queries() []query {
	typical := s.nodes / 2
	qs := []query{
		{"out/hops=1", typical, false, 1},
		{"out/hops=2", typical, false, 2},
		{"out/hops=3", typical, false, 3},
		{"out/hops=1..5", typical, false, 5},
	}
	if s.zipf {
		qs = append(qs, query{"hub-in/hops=1", 0, true, 1}, query{"hub-in/hops=2", 0, true, 2})
	}
	return qs
}

// engine runs a traversal and returns the number of distinct nodes reached.
type engine interface {
	run(q query) (int, error)
}

type built struct {
	engines map[string]engine
}

var (
	dir      string
	cacheMu  sync.Mutex
	cache    = map[string]*built{}
	extPath  = os.Getenv("GRAPHQLITE_EXT")
	register sync.Once
)

func TestMain(m *testing.M) {
	var err error
	dir, err = os.MkdirTemp("", "graph-compare-")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// load builds spec in every engine once per process and checks that all
// engines agree on each query's result size.
func load(b *testing.B, spec graphSpec) *built {
	b.Helper()
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if g, ok := cache[spec.name]; ok {
		return g
	}
	if extPath == "" {
		b.Skip("set GRAPHQLITE_EXT to the GraphQLite extension path")
	}
	register.Do(func() {
		sql.Register("sqlite3_graphqlite", &sqlite3.SQLiteDriver{Extensions: []string{extPath}})
	})

	edges := spec.generate()
	ours, err := loadOurs(b, spec, edges)
	if err != nil {
		b.Fatal(err)
	}
	cSQL, err := openCSQL(spec)
	if err != nil {
		b.Fatal(err)
	}
	gql, err := loadGraphQLite(b, spec, edges)
	if err != nil {
		b.Fatal(err)
	}
	g := &built{engines: map[string]engine{"ours": ours, "ours-sql-on-c": cSQL, "graphqlite": gql}}

	for _, q := range spec.queries() {
		counts := map[string]int{}
		for name, e := range g.engines {
			n, err := e.run(q)
			if err != nil {
				b.Fatalf("%s %s: %v", name, q.name, err)
			}
			counts[name] = n
		}
		b.Logf("%s %s: reached %v", spec.name, q.name, counts)
		if counts["ours"] != counts["graphqlite"] || counts["ours"] != counts["ours-sql-on-c"] {
			b.Fatalf("%s %s: engines disagree: %v", spec.name, q.name, counts)
		}
	}
	cache[spec.name] = g
	return g
}

func benchSpec(b *testing.B, spec graphSpec) {
	g := load(b, spec)
	for _, q := range spec.queries() {
		for _, name := range []string{"ours", "ours-sql-on-c", "graphqlite"} {
			e := g.engines[name]
			b.Run(q.name+"/"+name, func(b *testing.B) {
				for range b.N {
					if _, err := e.run(q); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkRandom10k(b *testing.B)    { benchSpec(b, random10k) }
func BenchmarkPowerLaw100k(b *testing.B) { benchSpec(b, powerLaw100) }

// ours uses the go-sqlite-graph API.
type ours struct{ g *graph.Graph }

func loadOurs(b *testing.B, spec graphSpec, edges [][2]int) (*ours, error) {
	ctx := context.Background()
	g, err := graph.Open("file:"+filepath.Join(dir, spec.name+"-ours.db"), &graph.Options{PoolSize: 2})
	if err != nil {
		return nil, err
	}
	start := time.Now()
	tx, err := g.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ids := make([]int64, spec.nodes)
	for i := range spec.nodes {
		n := &graph.Node{Name: fmt.Sprintf("n%d", i), Labels: []string{"Node"}}
		if err := tx.CreateNode(ctx, n); err != nil {
			return nil, err
		}
		ids[i] = n.ID
	}
	for _, e := range edges {
		if err := tx.CreateEdge(ctx, &graph.Edge{SourceID: ids[e[0]], TargetID: ids[e[1]], Type: "LINK"}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	b.Logf("%s ours: loaded in %v", spec.name, time.Since(start))
	return &ours{g: g}, nil
}

func (o *ours) run(q query) (int, error) {
	dir := graph.Outgoing
	if q.in {
		dir = graph.Incoming
	}
	res, err := o.g.Match("Node").
		Where("name", "=", fmt.Sprintf("n%d", q.start)).
		RelatedDir("LINK", dir, 1, q.maxHops).
		Return("name").
		Run(context.Background())
	if err != nil {
		return 0, err
	}
	return res.Len(), nil
}

// cSQL runs the SQL go-sqlite-graph generates against the same database file
// through C SQLite, isolating the SQLite engine from the library's design.
type cSQL struct {
	db    *sql.DB
	stmts map[query]*sql.Stmt
}

func openCSQL(spec graphSpec) (*cSQL, error) {
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, spec.name+"-ours.db")+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &cSQL{db: db, stmts: map[query]*sql.Stmt{}}, nil
}

// oursSQL mirrors go-sqlite-graph's compiled SQL for
// Match("Node").Where("name", "=", ?).RelatedDir("LINK", dir, 1, maxHops).Return("name").
func oursSQL(q query) string {
	near, far := "source_id", "target_id"
	if q.in {
		near, far = far, near
	}
	start := "WITH RECURSIVE s0(node_id) AS (SELECT n.id FROM nodes n JOIN node_labels nl ON nl.node_id = n.id WHERE +nl.label = 'Node' AND n.name = ?)"
	var step string
	if q.maxHops == 1 {
		step = fmt.Sprintf(", s1(node_id) AS (SELECT e.%s FROM s0 p CROSS JOIN edges e ON e.%s = p.node_id AND e.type = 'LINK')", far, near)
	} else {
		step = fmt.Sprintf(", w1(node_id, depth) AS (SELECT node_id, 0 FROM s0 UNION SELECT e.%s, p.depth + 1 FROM w1 p CROSS JOIN edges e ON e.%s = p.node_id AND e.type = 'LINK' WHERE p.depth < %d), s1(node_id) AS (SELECT node_id FROM w1 WHERE depth >= 1)", far, near, q.maxHops)
	}
	return start + step + " SELECT n.id, n.name FROM (SELECT DISTINCT node_id FROM s1) s CROSS JOIN nodes n ON n.id = s.node_id"
}

func (c *cSQL) run(q query) (int, error) {
	stmt, ok := c.stmts[q]
	if !ok {
		var err error
		if stmt, err = c.db.Prepare(oursSQL(q)); err != nil {
			return 0, err
		}
		c.stmts[q] = stmt
	}
	return countRows(stmt.Query(fmt.Sprintf("n%d", q.start)))
}

// graphQLite runs Cypher through the GraphQLite extension.
type graphQLite struct {
	db   *sql.DB
	stmt *sql.Stmt
}

func loadGraphQLite(b *testing.B, spec graphSpec, edges [][2]int) (*graphQLite, error) {
	db, err := sql.Open("sqlite3_graphqlite", "file:"+filepath.Join(dir, spec.name+"-graphqlite.db")+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	// Mirrors GraphQLite's Python insert_nodes_bulk/insert_edges_bulk, its
	// fastest documented load path.
	start := time.Now()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.Exec("INSERT INTO property_keys (key) VALUES ('name')")
	if err != nil {
		return nil, err
	}
	nameKey, _ := res.LastInsertId()
	insNode, _ := tx.Prepare("INSERT INTO nodes DEFAULT VALUES")
	insLabel, _ := tx.Prepare("INSERT INTO node_labels (node_id, label) VALUES (?, 'Node')")
	insName, _ := tx.Prepare("INSERT INTO node_props_text (node_id, key_id, value) VALUES (?, ?, ?)")
	insEdge, _ := tx.Prepare("INSERT INTO edges (source_id, target_id, type) VALUES (?, ?, 'LINK')")
	ids := make([]int64, spec.nodes)
	for i := range spec.nodes {
		r, err := insNode.Exec()
		if err != nil {
			return nil, err
		}
		ids[i], _ = r.LastInsertId()
		if _, err := insLabel.Exec(ids[i]); err != nil {
			return nil, err
		}
		if _, err := insName.Exec(ids[i], nameKey, fmt.Sprintf("n%d", i)); err != nil {
			return nil, err
		}
	}
	for _, e := range edges {
		if _, err := insEdge.Exec(ids[e[0]], ids[e[1]]); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	b.Logf("%s graphqlite: loaded in %v", spec.name, time.Since(start))

	stmt, err := db.Prepare("SELECT c0 FROM cypher_rows(?, ?)")
	if err != nil {
		return nil, err
	}
	return &graphQLite{db: db, stmt: stmt}, nil
}

func cypherFor(q query) string {
	rel := "[:LINK]"
	if q.maxHops > 1 {
		rel = fmt.Sprintf("[:LINK*1..%d]", q.maxHops)
	}
	if q.in {
		return "MATCH (a:Node {name: $name})<-" + rel + "-(b) RETURN DISTINCT b.name"
	}
	return "MATCH (a:Node {name: $name})-" + rel + "->(b) RETURN DISTINCT b.name"
}

func (g *graphQLite) run(q query) (int, error) {
	params := fmt.Sprintf(`{"name": "n%d"}`, q.start)
	n, err := countRows(g.stmt.Query(cypherFor(q), params))
	if err != nil {
		return 0, fmt.Errorf("cypher %q: %w", cypherFor(q), err)
	}
	return n, nil
}

func countRows(rows *sql.Rows, err error) (int, error) {
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	dest := make([]any, len(cols))
	for i := range dest {
		dest[i] = new(any)
	}
	n := 0
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			return 0, err
		}
		n++
	}
	return n, rows.Err()
}

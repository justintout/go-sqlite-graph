package graph

import (
	"context"
	"fmt"
	"testing"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func openTestGraph(t *testing.T) *Graph {
	t.Helper()
	// Use a shared-cache in-memory DB so all pool connections share the same data.
	uri := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := Open(uri, &Options{PoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

func TestOpen(t *testing.T) {
	g := openTestGraph(t)
	ctx := context.Background()

	conn, err := g.conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer g.put(conn)

	var tables []string
	err = sqlitex.Execute(conn, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;",
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				tables = append(tables, stmt.ColumnText(0))
				return nil
			},
		})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{"nodes": true, "node_labels": true, "edges": true}
	for name := range want {
		found := false
		for _, tbl := range tables {
			if tbl == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected table %q to exist, got tables: %v", name, tables)
		}
	}
}

func TestOpenManualMigrate(t *testing.T) {
	f := false
	uri := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := Open(uri, &Options{AutoMigrate: &f, PoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })

	ctx := context.Background()

	// Run migration
	if err := g.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	conn, err := g.conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer g.put(conn)

	var count int64
	err = sqlitex.Execute(conn, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='nodes';",
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				count = stmt.ColumnInt64(0)
				return nil
			},
		})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Error("nodes table should exist after migration")
	}
}

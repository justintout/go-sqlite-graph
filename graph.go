package graph

import (
	"context"
	"fmt"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitemigration"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Options configures graph database opening behavior.
type Options struct {
	// PoolSize sets the connection pool size. Defaults to 10 if <= 0.
	PoolSize int
	// AutoMigrate controls whether Open() automatically runs schema migrations.
	// Defaults to true. Set to a pointer to false to disable.
	AutoMigrate *bool
}

// Graph is the main handle to a labeled property graph stored in SQLite.
type Graph struct {
	migPool *sqlitemigration.Pool
	rawPool *sqlitex.Pool
	uri     string
}

func prepareConn(conn *sqlite.Conn) error {
	// PRAGMAs must be executed outside transactions (synchronous, journal_mode),
	// so we use individual ExecuteTransient calls rather than ExecuteScript.
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA cache_size=-64000;",
		"PRAGMA mmap_size=268435456;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, p := range pragmas {
		if err := sqlitex.ExecuteTransient(conn, p, nil); err != nil {
			return fmt.Errorf("graph: %s: %w", p, err)
		}
	}
	return nil
}

// Open opens (or creates) a graph database at the given path.
// By default it auto-migrates the schema on first connection.
func Open(uri string, opts *Options) (*Graph, error) {
	if opts == nil {
		opts = &Options{}
	}
	poolSize := opts.PoolSize
	if poolSize <= 0 {
		poolSize = 10
	}

	autoMigrate := true
	if opts.AutoMigrate != nil {
		autoMigrate = *opts.AutoMigrate
	}

	g := &Graph{uri: uri}

	if autoMigrate {
		pool := sqlitemigration.NewPool(uri, graphSchema(), sqlitemigration.Options{
			PoolSize: poolSize,
			Flags:    sqlite.OpenReadWrite | sqlite.OpenCreate | sqlite.OpenURI,
			PrepareConn: func(conn *sqlite.Conn) error {
				return prepareConn(conn)
			},
		})
		g.migPool = pool

		// Run ANALYZE after migration to populate sqlite_stat1 for the query planner.
		conn, err := pool.Get(context.Background())
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("graph: post-migration analyze: %w", err)
		}
		err = maybeAnalyze(conn)
		pool.Put(conn)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("graph: post-migration analyze: %w", err)
		}
	} else {
		pool, err := sqlitex.NewPool(uri, sqlitex.PoolOptions{
			PoolSize: poolSize,
			Flags:    sqlite.OpenReadWrite | sqlite.OpenCreate | sqlite.OpenURI,
			PrepareConn: func(conn *sqlite.Conn) error {
				return prepareConn(conn)
			},
		})
		if err != nil {
			return nil, fmt.Errorf("graph: open pool: %w", err)
		}
		g.rawPool = pool
	}

	return g, nil
}

// Migrate explicitly applies schema migrations. Only needed when AutoMigrate is false.
func (g *Graph) Migrate(ctx context.Context) error {
	conn, err := g.conn(ctx)
	if err != nil {
		return err
	}
	defer g.put(conn)
	return sqlitemigration.Migrate(ctx, conn, graphSchema())
}

// Close closes the database connection pool.
func (g *Graph) Close() error {
	if g.migPool != nil {
		return g.migPool.Close()
	}
	if g.rawPool != nil {
		return g.rawPool.Close()
	}
	return nil
}

func (g *Graph) conn(ctx context.Context) (*sqlite.Conn, error) {
	if g.migPool != nil {
		conn, err := g.migPool.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("graph: could not get connection from pool: %w", err)
		}
		return conn, nil
	}
	conn, err := g.rawPool.Take(ctx)
	if err != nil {
		return nil, fmt.Errorf("graph: could not get connection from pool: %w", err)
	}
	return conn, nil
}

// maybeAnalyze runs ANALYZE if sqlite_stat1 is empty or missing,
// populating statistics for the query planner.
func maybeAnalyze(conn *sqlite.Conn) error {
	var hasStats bool
	err := sqlitex.Execute(conn, "SELECT 1 FROM sqlite_stat1 LIMIT 1", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			hasStats = true
			return nil
		},
	})
	if err != nil {
		// sqlite_stat1 doesn't exist yet — ANALYZE will create it.
		hasStats = false
	}
	if !hasStats {
		return sqlitex.ExecuteTransient(conn, "ANALYZE", nil)
	}
	return nil
}

func (g *Graph) put(conn *sqlite.Conn) {
	if g.migPool != nil {
		g.migPool.Put(conn)
		return
	}
	g.rawPool.Put(conn)
}

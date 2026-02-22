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
	return sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil)
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

func (g *Graph) put(conn *sqlite.Conn) {
	if g.migPool != nil {
		g.migPool.Put(conn)
		return
	}
	g.rawPool.Put(conn)
}

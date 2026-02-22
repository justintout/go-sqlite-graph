package graph

import (
	"context"
	"fmt"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Edge represents a directed, typed relationship between two nodes.
type Edge struct {
	ID         int64
	SourceID   int64
	TargetID   int64
	Type       string
	Name       string
	CreatedAt  string
	UpdatedAt  string
	Properties map[string]any
}

// CreateEdge inserts a new edge between two nodes.
func (g *Graph) CreateEdge(ctx context.Context, e *Edge) error {
	conn, err := g.conn(ctx)
	if err != nil {
		return err
	}
	defer g.put(conn)
	return createEdgeInternal(conn, e)
}

// GetEdge retrieves an edge by ID.
func (g *Graph) GetEdge(ctx context.Context, id int64) (*Edge, error) {
	conn, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}
	defer g.put(conn)
	return getEdgeInternal(conn, id)
}

// UpdateEdge updates an edge's type, name, and/or properties.
func (g *Graph) UpdateEdge(ctx context.Context, e *Edge) error {
	conn, err := g.conn(ctx)
	if err != nil {
		return err
	}
	defer g.put(conn)
	return updateEdgeInternal(conn, e)
}

// DeleteEdge removes an edge by ID.
func (g *Graph) DeleteEdge(ctx context.Context, id int64) error {
	conn, err := g.conn(ctx)
	if err != nil {
		return err
	}
	defer g.put(conn)
	return deleteEdgeInternal(conn, id)
}

func createEdgeInternal(conn *sqlite.Conn, e *Edge) error {
	props, err := MarshalProperties(e.Properties)
	if err != nil {
		return fmt.Errorf("graph: marshal properties: %w", err)
	}

	err = sqlitex.Execute(conn,
		"INSERT INTO edges (source_id, target_id, type, name, properties) VALUES (?, ?, ?, ?, ?);",
		&sqlitex.ExecOptions{Args: []any{e.SourceID, e.TargetID, e.Type, e.Name, props}},
	)
	if err != nil {
		return fmt.Errorf("graph: insert edge: %w", err)
	}

	e.ID = conn.LastInsertRowID()

	// Read back timestamps
	err = sqlitex.Execute(conn,
		"SELECT created_at, updated_at FROM edges WHERE id = ?;",
		&sqlitex.ExecOptions{
			Args: []any{e.ID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				e.CreatedAt = stmt.ColumnText(0)
				e.UpdatedAt = stmt.ColumnText(1)
				return nil
			},
		},
	)
	if err != nil {
		return fmt.Errorf("graph: read timestamps: %w", err)
	}

	return nil
}

func getEdgeInternal(conn *sqlite.Conn, id int64) (*Edge, error) {
	e := &Edge{ID: id}
	found := false

	err := sqlitex.Execute(conn,
		"SELECT source_id, target_id, type, name, created_at, updated_at, properties FROM edges WHERE id = ?;",
		&sqlitex.ExecOptions{
			Args: []any{id},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				e.SourceID = stmt.ColumnInt64(0)
				e.TargetID = stmt.ColumnInt64(1)
				e.Type = stmt.ColumnText(2)
				e.Name = stmt.ColumnText(3)
				e.CreatedAt = stmt.ColumnText(4)
				e.UpdatedAt = stmt.ColumnText(5)
				var err error
				e.Properties, err = UnmarshalProperties(stmt.ColumnText(6))
				return err
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("graph: get edge: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("graph: edge %d not found", id)
	}

	return e, nil
}

func updateEdgeInternal(conn *sqlite.Conn, e *Edge) error {
	props, err := MarshalProperties(e.Properties)
	if err != nil {
		return fmt.Errorf("graph: marshal properties: %w", err)
	}

	err = sqlitex.Execute(conn,
		"UPDATE edges SET type = ?, name = ?, properties = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?;",
		&sqlitex.ExecOptions{Args: []any{e.Type, e.Name, props, e.ID}},
	)
	if err != nil {
		return fmt.Errorf("graph: update edge: %w", err)
	}
	if conn.Changes() == 0 {
		return fmt.Errorf("graph: edge %d not found", e.ID)
	}

	// Read back updated_at
	err = sqlitex.Execute(conn,
		"SELECT updated_at FROM edges WHERE id = ?;",
		&sqlitex.ExecOptions{
			Args: []any{e.ID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				e.UpdatedAt = stmt.ColumnText(0)
				return nil
			},
		},
	)
	if err != nil {
		return fmt.Errorf("graph: read updated_at: %w", err)
	}

	return nil
}

func deleteEdgeInternal(conn *sqlite.Conn, id int64) error {
	err := sqlitex.Execute(conn,
		"DELETE FROM edges WHERE id = ?;",
		&sqlitex.ExecOptions{Args: []any{id}},
	)
	if err != nil {
		return fmt.Errorf("graph: delete edge: %w", err)
	}
	if conn.Changes() == 0 {
		return fmt.Errorf("graph: edge %d not found", id)
	}
	return nil
}

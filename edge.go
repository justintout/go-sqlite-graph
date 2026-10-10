package graph

import (
	"context"
	"encoding/json"
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

// EdgesBetween returns the edges whose source and target are both in nodeIDs,
// in no guaranteed order. With types given, it returns only edges of those
// types.
func (g *Graph) EdgesBetween(ctx context.Context, nodeIDs []int64, types ...string) ([]*Edge, error) {
	conn, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}
	defer g.put(conn)
	return edgesBetweenInternal(conn, nodeIDs, types)
}

func createEdgeInternal(conn *sqlite.Conn, e *Edge) error {
	props, err := MarshalProperties(e.Properties)
	if err != nil {
		return fmt.Errorf("graph: marshal properties: %w", err)
	}

	now := timestamp()
	err = sqlitex.Execute(conn,
		"INSERT INTO edges (source_id, target_id, type, name, properties, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?);",
		&sqlitex.ExecOptions{Args: []any{e.SourceID, e.TargetID, e.Type, e.Name, props, now, now}},
	)
	if err != nil {
		return fmt.Errorf("graph: insert edge: %w", err)
	}
	e.ID = conn.LastInsertRowID()
	e.CreatedAt, e.UpdatedAt = now, now

	return nil
}

const edgeCols = "e.id, e.source_id, e.target_id, e.type, e.name, e.created_at, e.updated_at, e.properties"

// scanEdge reads a row selected with edgeCols.
func scanEdge(stmt *sqlite.Stmt) (*Edge, error) {
	e := &Edge{
		ID:        stmt.ColumnInt64(0),
		SourceID:  stmt.ColumnInt64(1),
		TargetID:  stmt.ColumnInt64(2),
		Type:      stmt.ColumnText(3),
		Name:      stmt.ColumnText(4),
		CreatedAt: stmt.ColumnText(5),
		UpdatedAt: stmt.ColumnText(6),
	}
	var err error
	e.Properties, err = UnmarshalProperties(stmt.ColumnText(7))
	return e, err
}

func getEdgeInternal(conn *sqlite.Conn, id int64) (*Edge, error) {
	var e *Edge
	err := sqlitex.Execute(conn,
		"SELECT "+edgeCols+" FROM edges e WHERE e.id = ?;",
		&sqlitex.ExecOptions{
			Args: []any{id},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				var err error
				e, err = scanEdge(stmt)
				return err
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("graph: get edge: %w", err)
	}
	if e == nil {
		return nil, fmt.Errorf("graph: edge %d not found", id)
	}
	return e, nil
}

// edgesBetweenInternal binds the ID and type lists as JSON arrays, so the SQL
// text does not vary with their lengths. Each node's outgoing edges are read
// from the (source_id, type, target_id) index, and the target is checked
// against the set. The unary + on target_id stops the planner from seeking
// the target index once per (source, target) pair, which is quadratic in the
// number of nodes.
func edgesBetweenInternal(conn *sqlite.Conn, nodeIDs []int64, types []string) ([]*Edge, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	ids, err := json.Marshal(nodeIDs)
	if err != nil {
		return nil, fmt.Errorf("graph: encode node IDs: %w", err)
	}
	query := "SELECT " + edgeCols + " FROM (SELECT DISTINCT value AS id FROM json_each(?)) s" +
		" CROSS JOIN edges e ON e.source_id = s.id" +
		" WHERE +e.target_id IN (SELECT value FROM json_each(?))"
	args := []any{string(ids), string(ids)}
	if len(types) > 0 {
		t, err := json.Marshal(types)
		if err != nil {
			return nil, fmt.Errorf("graph: encode edge types: %w", err)
		}
		query += " AND e.type IN (SELECT value FROM json_each(?))"
		args = append(args, string(t))
	}

	var edges []*Edge
	err = sqlitex.Execute(conn, query+";", &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			e, err := scanEdge(stmt)
			if err != nil {
				return err
			}
			edges = append(edges, e)
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("graph: edges between: %w", err)
	}
	return edges, nil
}

func updateEdgeInternal(conn *sqlite.Conn, e *Edge) error {
	props, err := MarshalProperties(e.Properties)
	if err != nil {
		return fmt.Errorf("graph: marshal properties: %w", err)
	}

	now := timestamp()
	err = sqlitex.Execute(conn,
		"UPDATE edges SET type = ?, name = ?, properties = ?, updated_at = ? WHERE id = ?;",
		&sqlitex.ExecOptions{Args: []any{e.Type, e.Name, props, now, e.ID}},
	)
	if err != nil {
		return fmt.Errorf("graph: update edge: %w", err)
	}
	if conn.Changes() == 0 {
		return fmt.Errorf("graph: edge %d not found", e.ID)
	}
	e.UpdatedAt = now

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

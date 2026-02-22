package graph

import (
	"context"
	"fmt"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Node represents a graph node with labels and properties.
type Node struct {
	ID         int64
	Name       string
	Labels     []string
	CreatedAt  string
	UpdatedAt  string
	Properties map[string]any
}

// CreateNode inserts a new node into the graph.
func (g *Graph) CreateNode(ctx context.Context, n *Node) error {
	conn, err := g.conn(ctx)
	if err != nil {
		return err
	}
	defer g.put(conn)
	return createNodeInternal(conn, n)
}

// GetNode retrieves a node by ID, including its labels.
func (g *Graph) GetNode(ctx context.Context, id int64) (*Node, error) {
	conn, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}
	defer g.put(conn)
	return getNodeInternal(conn, id)
}

// UpdateNode updates a node's name and/or properties. Sets updated_at.
func (g *Graph) UpdateNode(ctx context.Context, n *Node) error {
	conn, err := g.conn(ctx)
	if err != nil {
		return err
	}
	defer g.put(conn)
	return updateNodeInternal(conn, n)
}

// DeleteNode removes a node and (via CASCADE) its labels and connected edges.
func (g *Graph) DeleteNode(ctx context.Context, id int64) error {
	conn, err := g.conn(ctx)
	if err != nil {
		return err
	}
	defer g.put(conn)
	return deleteNodeInternal(conn, id)
}

// AddLabels adds labels to an existing node (idempotent).
func (g *Graph) AddLabels(ctx context.Context, nodeID int64, labels ...string) error {
	conn, err := g.conn(ctx)
	if err != nil {
		return err
	}
	defer g.put(conn)
	return addLabelsInternal(conn, nodeID, labels)
}

// RemoveLabels removes labels from a node.
func (g *Graph) RemoveLabels(ctx context.Context, nodeID int64, labels ...string) error {
	conn, err := g.conn(ctx)
	if err != nil {
		return err
	}
	defer g.put(conn)
	return removeLabelsInternal(conn, nodeID, labels)
}

func createNodeInternal(conn *sqlite.Conn, n *Node) (err error) {
	defer sqlitex.Save(conn)(&err)

	props, err := MarshalProperties(n.Properties)
	if err != nil {
		return fmt.Errorf("graph: marshal properties: %w", err)
	}

	err = sqlitex.Execute(conn,
		"INSERT INTO nodes (name, properties) VALUES (?, ?);",
		&sqlitex.ExecOptions{Args: []any{n.Name, props}},
	)
	if err != nil {
		return fmt.Errorf("graph: insert node: %w", err)
	}

	n.ID = conn.LastInsertRowID()

	for _, label := range n.Labels {
		err = sqlitex.Execute(conn,
			"INSERT OR IGNORE INTO node_labels (node_id, label) VALUES (?, ?);",
			&sqlitex.ExecOptions{Args: []any{n.ID, label}},
		)
		if err != nil {
			return fmt.Errorf("graph: insert label %q: %w", label, err)
		}
	}

	// Read back timestamps
	err = sqlitex.Execute(conn,
		"SELECT created_at, updated_at FROM nodes WHERE id = ?;",
		&sqlitex.ExecOptions{
			Args: []any{n.ID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				n.CreatedAt = stmt.ColumnText(0)
				n.UpdatedAt = stmt.ColumnText(1)
				return nil
			},
		},
	)
	if err != nil {
		return fmt.Errorf("graph: read timestamps: %w", err)
	}

	return nil
}

func getNodeInternal(conn *sqlite.Conn, id int64) (*Node, error) {
	n := &Node{ID: id}
	found := false

	err := sqlitex.Execute(conn,
		"SELECT name, created_at, updated_at, properties FROM nodes WHERE id = ?;",
		&sqlitex.ExecOptions{
			Args: []any{id},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				n.Name = stmt.ColumnText(0)
				n.CreatedAt = stmt.ColumnText(1)
				n.UpdatedAt = stmt.ColumnText(2)
				var err error
				n.Properties, err = UnmarshalProperties(stmt.ColumnText(3))
				return err
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("graph: get node: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("graph: node %d not found", id)
	}

	err = sqlitex.Execute(conn,
		"SELECT label FROM node_labels WHERE node_id = ? ORDER BY label;",
		&sqlitex.ExecOptions{
			Args: []any{id},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				n.Labels = append(n.Labels, stmt.ColumnText(0))
				return nil
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("graph: get labels: %w", err)
	}

	return n, nil
}

func updateNodeInternal(conn *sqlite.Conn, n *Node) (err error) {
	defer sqlitex.Save(conn)(&err)

	props, err := MarshalProperties(n.Properties)
	if err != nil {
		return fmt.Errorf("graph: marshal properties: %w", err)
	}

	err = sqlitex.Execute(conn,
		"UPDATE nodes SET name = ?, properties = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?;",
		&sqlitex.ExecOptions{Args: []any{n.Name, props, n.ID}},
	)
	if err != nil {
		return fmt.Errorf("graph: update node: %w", err)
	}
	if conn.Changes() == 0 {
		return fmt.Errorf("graph: node %d not found", n.ID)
	}

	// Sync labels: delete all, re-insert
	err = sqlitex.Execute(conn,
		"DELETE FROM node_labels WHERE node_id = ?;",
		&sqlitex.ExecOptions{Args: []any{n.ID}},
	)
	if err != nil {
		return fmt.Errorf("graph: delete labels: %w", err)
	}

	for _, label := range n.Labels {
		err = sqlitex.Execute(conn,
			"INSERT INTO node_labels (node_id, label) VALUES (?, ?);",
			&sqlitex.ExecOptions{Args: []any{n.ID, label}},
		)
		if err != nil {
			return fmt.Errorf("graph: insert label %q: %w", label, err)
		}
	}

	// Read back updated_at
	err = sqlitex.Execute(conn,
		"SELECT updated_at FROM nodes WHERE id = ?;",
		&sqlitex.ExecOptions{
			Args: []any{n.ID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				n.UpdatedAt = stmt.ColumnText(0)
				return nil
			},
		},
	)
	if err != nil {
		return fmt.Errorf("graph: read updated_at: %w", err)
	}

	return nil
}

func deleteNodeInternal(conn *sqlite.Conn, id int64) error {
	err := sqlitex.Execute(conn,
		"DELETE FROM nodes WHERE id = ?;",
		&sqlitex.ExecOptions{Args: []any{id}},
	)
	if err != nil {
		return fmt.Errorf("graph: delete node: %w", err)
	}
	if conn.Changes() == 0 {
		return fmt.Errorf("graph: node %d not found", id)
	}
	return nil
}

func addLabelsInternal(conn *sqlite.Conn, nodeID int64, labels []string) (err error) {
	defer sqlitex.Save(conn)(&err)
	for _, label := range labels {
		err = sqlitex.Execute(conn,
			"INSERT OR IGNORE INTO node_labels (node_id, label) VALUES (?, ?);",
			&sqlitex.ExecOptions{Args: []any{nodeID, label}},
		)
		if err != nil {
			return fmt.Errorf("graph: add label %q: %w", label, err)
		}
	}
	return nil
}

func removeLabelsInternal(conn *sqlite.Conn, nodeID int64, labels []string) (err error) {
	defer sqlitex.Save(conn)(&err)
	for _, label := range labels {
		err = sqlitex.Execute(conn,
			"DELETE FROM node_labels WHERE node_id = ? AND label = ?;",
			&sqlitex.ExecOptions{Args: []any{nodeID, label}},
		)
		if err != nil {
			return fmt.Errorf("graph: remove label %q: %w", label, err)
		}
	}
	return nil
}

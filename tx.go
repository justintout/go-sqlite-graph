package graph

import (
	"context"
	"fmt"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Tx represents an explicit graph transaction.
type Tx struct {
	g    *Graph
	conn *sqlite.Conn
	done bool
	end  func(*error)
}

// BeginTx starts a new transaction and acquires a connection from the pool.
func (g *Graph) BeginTx(ctx context.Context) (*Tx, error) {
	conn, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}

	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		g.put(conn)
		return nil, fmt.Errorf("graph: begin tx: %w", err)
	}

	return &Tx{g: g, conn: conn, end: end}, nil
}

// Commit commits the transaction and returns the connection to the pool.
func (tx *Tx) Commit() error {
	if tx.done {
		return fmt.Errorf("graph: transaction already finished")
	}
	tx.done = true
	var err error
	tx.end(&err)
	tx.g.put(tx.conn)
	if err != nil {
		return fmt.Errorf("graph: commit: %w", err)
	}
	return nil
}

// Rollback rolls back the transaction and returns the connection to the pool.
func (tx *Tx) Rollback() error {
	if tx.done {
		return nil // idempotent rollback for defer patterns
	}
	tx.done = true
	rollbackErr := fmt.Errorf("rollback")
	tx.end(&rollbackErr)
	tx.g.put(tx.conn)
	return nil
}

// CreateNode creates a node within this transaction.
func (tx *Tx) CreateNode(ctx context.Context, n *Node) error {
	return createNodeInternal(tx.conn, n)
}

// GetNode retrieves a node within this transaction.
func (tx *Tx) GetNode(ctx context.Context, id int64) (*Node, error) {
	return getNodeInternal(tx.conn, id)
}

// UpdateNode updates a node within this transaction.
func (tx *Tx) UpdateNode(ctx context.Context, n *Node) error {
	return updateNodeInternal(tx.conn, n)
}

// DeleteNode deletes a node within this transaction.
func (tx *Tx) DeleteNode(ctx context.Context, id int64) error {
	return deleteNodeInternal(tx.conn, id)
}

// AddLabels adds labels within this transaction.
func (tx *Tx) AddLabels(ctx context.Context, nodeID int64, labels ...string) error {
	return addLabelsInternal(tx.conn, nodeID, labels)
}

// RemoveLabels removes labels within this transaction.
func (tx *Tx) RemoveLabels(ctx context.Context, nodeID int64, labels ...string) error {
	return removeLabelsInternal(tx.conn, nodeID, labels)
}

// CreateEdge creates an edge within this transaction.
func (tx *Tx) CreateEdge(ctx context.Context, e *Edge) error {
	return createEdgeInternal(tx.conn, e)
}

// GetEdge retrieves an edge within this transaction.
func (tx *Tx) GetEdge(ctx context.Context, id int64) (*Edge, error) {
	return getEdgeInternal(tx.conn, id)
}

// UpdateEdge updates an edge within this transaction.
func (tx *Tx) UpdateEdge(ctx context.Context, e *Edge) error {
	return updateEdgeInternal(tx.conn, e)
}

// DeleteEdge deletes an edge within this transaction.
func (tx *Tx) DeleteEdge(ctx context.Context, id int64) error {
	return deleteEdgeInternal(tx.conn, id)
}

// Match starts a query within this transaction.
func (tx *Tx) Match(label string) *Query {
	return &Query{
		conn:       tx.conn,
		matchLabel: label,
	}
}

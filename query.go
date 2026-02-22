package graph

import (
	"context"
	"fmt"
	"strings"

	"zombiezen.com/go/sqlite"
)

// Direction indicates edge traversal direction.
type Direction int

const (
	Outgoing Direction = iota // source -> target
	Incoming                  // target -> source
	Both                      // either direction
)

// MaxHops is the maximum allowed hop depth for traversal queries.
const MaxHops = 10

// validOps is the whitelist of allowed SQL operators.
var validOps = map[string]bool{
	"=": true, "!=": true, "<>": true,
	">": true, "<": true, ">=": true, "<=": true,
	"LIKE": true, "NOT LIKE": true,
	"IN": true, "NOT IN": true,
	"IS": true, "IS NOT": true,
}

func validateOp(op string) error {
	if !validOps[strings.ToUpper(op)] {
		return fmt.Errorf("graph: invalid operator %q", op)
	}
	return nil
}

type whereClause struct {
	field  string
	op     string
	value  any
	isJSON bool
}

type relStep struct {
	edgeType  string
	direction Direction
	minHops   int
	maxHops   int
	wheres    []whereClause
}

// Query is a fluent builder for graph traversal queries.
type Query struct {
	g          *Graph
	conn       *sqlite.Conn // set when running inside a Tx
	matchLabel string
	wheres     []whereClause
	rels       []relStep
	returnCols []string
	limitVal   int
	offsetVal  int
	err        error // captures builder errors
}

// Match starts a query by filtering nodes with the given label.
func (g *Graph) Match(label string) *Query {
	return &Query{
		g:          g,
		matchLabel: label,
	}
}

// Where adds a column-level filter on the starting node set.
func (q *Query) Where(field, op string, value any) *Query {
	if err := validateOp(op); err != nil {
		q.err = err
		return q
	}
	q.wheres = append(q.wheres, whereClause{field: field, op: strings.ToUpper(op), value: value})
	return q
}

// WhereJSON adds a JSON property filter on the starting node set.
func (q *Query) WhereJSON(path, op string, value any) *Query {
	if err := validateOp(op); err != nil {
		q.err = err
		return q
	}
	q.wheres = append(q.wheres, whereClause{field: path, op: strings.ToUpper(op), value: value, isJSON: true})
	return q
}

// Related adds a relationship traversal step (outgoing direction).
func (q *Query) Related(edgeType string, minHops, maxHops int) *Query {
	return q.RelatedDir(edgeType, Outgoing, minHops, maxHops)
}

// RelatedDir adds a relationship traversal step with explicit direction.
func (q *Query) RelatedDir(edgeType string, dir Direction, minHops, maxHops int) *Query {
	if minHops < 1 {
		q.err = fmt.Errorf("graph: minHops must be >= 1, got %d", minHops)
		return q
	}
	if maxHops < minHops {
		q.err = fmt.Errorf("graph: maxHops (%d) must be >= minHops (%d)", maxHops, minHops)
		return q
	}
	if maxHops > MaxHops {
		q.err = fmt.Errorf("graph: maxHops (%d) exceeds limit of %d", maxHops, MaxHops)
		return q
	}
	q.rels = append(q.rels, relStep{
		edgeType:  edgeType,
		direction: dir,
		minHops:   minHops,
		maxHops:   maxHops,
	})
	return q
}

// WhereRel adds a column filter on nodes reached in the most recent Related() step.
func (q *Query) WhereRel(field, op string, value any) *Query {
	if err := validateOp(op); err != nil {
		q.err = err
		return q
	}
	if len(q.rels) == 0 {
		q.err = fmt.Errorf("graph: WhereRel called without a preceding Related()")
		return q
	}
	idx := len(q.rels) - 1
	q.rels[idx].wheres = append(q.rels[idx].wheres, whereClause{field: field, op: strings.ToUpper(op), value: value})
	return q
}

// WhereRelJSON adds a JSON property filter on nodes reached in the most recent Related() step.
func (q *Query) WhereRelJSON(path, op string, value any) *Query {
	if err := validateOp(op); err != nil {
		q.err = err
		return q
	}
	if len(q.rels) == 0 {
		q.err = fmt.Errorf("graph: WhereRelJSON called without a preceding Related()")
		return q
	}
	idx := len(q.rels) - 1
	q.rels[idx].wheres = append(q.rels[idx].wheres, whereClause{field: path, op: strings.ToUpper(op), value: value, isJSON: true})
	return q
}

// Return specifies which columns/properties to project in results.
// Known columns (id, name, created_at, updated_at) map directly.
// Other names are treated as JSON property paths.
func (q *Query) Return(cols ...string) *Query {
	q.returnCols = cols
	return q
}

// Limit sets a maximum number of results.
func (q *Query) Limit(n int) *Query {
	q.limitVal = n
	return q
}

// Offset sets a result offset for pagination.
func (q *Query) Offset(n int) *Query {
	q.offsetVal = n
	return q
}

// Run executes the query and returns results.
func (q *Query) Run(ctx context.Context) (*Result, error) {
	if q.err != nil {
		return nil, q.err
	}

	compiled, err := q.compile()
	if err != nil {
		return nil, err
	}

	var conn *sqlite.Conn
	if q.conn != nil {
		conn = q.conn
	} else {
		conn, err = q.g.conn(ctx)
		if err != nil {
			return nil, err
		}
		defer q.g.put(conn)
	}

	return compiled.execute(conn)
}

// Count executes the query and returns only the count of matching results.
func (q *Query) Count(ctx context.Context) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}

	compiled, err := q.compileCount()
	if err != nil {
		return 0, err
	}

	var conn *sqlite.Conn
	if q.conn != nil {
		conn = q.conn
	} else {
		conn, err = q.g.conn(ctx)
		if err != nil {
			return 0, err
		}
		defer q.g.put(conn)
	}

	return compiled.executeCount(conn)
}

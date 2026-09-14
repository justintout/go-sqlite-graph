package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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

// filterCols is the whitelist of node columns Where and WhereRel accept.
// Column names are written into the SQL text, so nothing else may pass.
var filterCols = map[string]bool{
	"id": true, "name": true, "created_at": true, "updated_at": true, "properties": true,
}

type whereClause struct {
	field  string
	op     string
	value  any
	isJSON bool
	// elem is a zero value of the slice element type for IN and NOT IN, whose
	// value is the slice encoded as a JSON array. It picks the property cast.
	elem any
}

// newWhere validates a filter on a column, or on a property path if isJSON.
func newWhere(field, op string, value any, isJSON bool) (whereClause, error) {
	if !isJSON && !filterCols[field] {
		return whereClause{}, fmt.Errorf("graph: invalid column %q; use WhereJSON for properties", field)
	}
	w := whereClause{field: field, op: strings.ToUpper(op), value: value, isJSON: isJSON}
	if !validOps[w.op] {
		return whereClause{}, fmt.Errorf("graph: invalid operator %q", op)
	}
	if w.op != "IN" && w.op != "NOT IN" {
		return w, nil
	}
	// The list is bound as one JSON array so the SQL text, and so the
	// connection's prepared statement cache, does not vary with its length.
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() == reflect.Uint8 {
		return whereClause{}, fmt.Errorf("graph: %s requires a slice value, got %T", w.op, value)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return whereClause{}, fmt.Errorf("graph: encode %s list: %w", w.op, err)
	}
	w.value = string(b)
	w.elem = reflect.Zero(v.Type().Elem()).Interface()
	return w, nil
}

type relStep struct {
	edgeType  string
	direction Direction
	minHops   int
	maxHops   int
	wheres    []whereClause
	// breadthFirst compiles the step with writeBFS.
	breadthFirst bool
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
// IN and NOT IN take a slice value.
func (q *Query) Where(field, op string, value any) *Query {
	return q.where(field, op, value, false)
}

// WhereJSON adds a JSON property filter on the starting node set.
// IN and NOT IN take a slice value.
func (q *Query) WhereJSON(path, op string, value any) *Query {
	return q.where(path, op, value, true)
}

func (q *Query) where(field, op string, value any, isJSON bool) *Query {
	w, err := newWhere(field, op, value, isJSON)
	if err != nil {
		q.err = err
		return q
	}
	q.wheres = append(q.wheres, w)
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

// BreadthFirst makes the most recent Related() step expand each reached node
// once. By default a multi-hop step deduplicates on (node, depth), so a node
// reachable at several depths is expanded at each of them. Results are the
// same either way.
//
// Breadth-first pays a fixed cost per hop level, so it helps only when the
// walk revisits many nodes, as on densely connected graphs with cycles. On a
// 500-node graph with 5 edges per node, a 10-hop step drops from 9.5ms to
// 2.0ms; on a 100-node chain it rises from 40µs to 243µs. Measure before
// choosing it. It requires minHops == 1: nodes reachable only at an exact
// larger depth are discarded by a visited set.
func (q *Query) BreadthFirst() *Query {
	if len(q.rels) == 0 {
		q.err = fmt.Errorf("graph: BreadthFirst called without a preceding Related()")
		return q
	}
	r := &q.rels[len(q.rels)-1]
	if r.minHops != 1 {
		q.err = fmt.Errorf("graph: BreadthFirst requires minHops of 1, got %d", r.minHops)
		return q
	}
	r.breadthFirst = r.maxHops > 1
	return q
}

// WhereRel adds a column filter on nodes reached in the most recent Related() step.
func (q *Query) WhereRel(field, op string, value any) *Query {
	return q.whereRel(field, op, value, false)
}

// WhereRelJSON adds a JSON property filter on nodes reached in the most recent Related() step.
func (q *Query) WhereRelJSON(path, op string, value any) *Query {
	return q.whereRel(path, op, value, true)
}

func (q *Query) whereRel(field, op string, value any, isJSON bool) *Query {
	w, err := newWhere(field, op, value, isJSON)
	if err != nil {
		q.err = err
		return q
	}
	if len(q.rels) == 0 {
		q.err = fmt.Errorf("graph: WhereRel called without a preceding Related()")
		return q
	}
	idx := len(q.rels) - 1
	q.rels[idx].wheres = append(q.rels[idx].wheres, w)
	return q
}

// Return specifies which columns/properties to project in results.
// Known columns (name, created_at, updated_at, properties) map to Node fields.
// Other names are treated as JSON property paths and set in Node.Properties
// under the name as given; paths missing from a node are omitted. Node.ID is
// always set. Fields not returned are left zero, and Properties is nil unless
// properties or a path is returned. Without Return, all columns are returned.
func (q *Query) Return(cols ...string) *Query {
	for _, c := range cols {
		if c == "" {
			q.err = fmt.Errorf("graph: Return called with an empty column name")
			return q
		}
	}
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

	compiled := q.compile(false)

	conn := q.conn
	if conn == nil {
		var err error
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

	compiled := q.compile(true)

	conn := q.conn
	if conn == nil {
		var err error
		conn, err = q.g.conn(ctx)
		if err != nil {
			return 0, err
		}
		defer q.g.put(conn)
	}

	return compiled.executeCount(conn)
}

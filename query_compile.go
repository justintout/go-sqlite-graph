package graph

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type compiledQuery struct {
	sql  string
	args []any
	// cols lists the selected columns after n.id, as Return names.
	cols []string
}

// sqlBuilder accumulates SQL text and its bound arguments. It writes plain
// strings rather than using fmt, which allocated for every formatted value and
// made compiling a large share of a small query's cost.
type sqlBuilder struct {
	strings.Builder
	args []any
}

func (b *sqlBuilder) w(parts ...string) {
	for _, p := range parts {
		b.WriteString(p)
	}
}

// allCols is the default projection. It omits labels: reading them runs a
// subquery and decodes a JSON array per node, which made traversals ~40%
// slower. Return("labels") and WithLabels select them.
var allCols = []string{"name", "created_at", "updated_at", "properties"}

var knownNodeCols = map[string]bool{"name": true, "labels": true, "created_at": true, "updated_at": true, "properties": true}

// labelsExpr selects a node's labels as a JSON array, sorted like GetNode.
const labelsExpr = "(SELECT json_group_array(label) FROM (SELECT label FROM node_labels WHERE node_id = n.id ORDER BY label))"

// compile builds the SQL for q. With count set, it selects COUNT(*) and
// ignores Limit and Offset.
//
// Traversals compile to a chain of CTEs, one per step, each yielding a set of
// node IDs. Nodes are read only for the final set. Selecting full rows through
// the joins would force SQLite to DISTINCT over the JSON properties text and to
// scan the nodes table to probe the reached set. The set is joined as a
// DISTINCT subquery rather than probed with IN, because IN builds a Bloom
// filter whose allocation costs an mmap syscall per query under modernc.
//
// Joins from a step's set use CROSS JOIN, which SQLite never reorders. The
// planner has no row estimates for CTEs and otherwise flattens the chain and
// scans a whole edge index, probing the previous step's set per edge.
//
// LIMIT and OFFSET are bound parameters because zombiezen caches prepared
// statements per connection by SQL text; interpolated values would grow that
// cache without bound when paginating. The clause is omitted when unused:
// a bound LIMIT makes SQLite re-prepare the statement on every execution.
func (q *Query) compile(count bool) *compiledQuery {
	c := &compiledQuery{}
	b := &sqlBuilder{args: make([]any, 0, 8)}
	b.Grow(512)

	if len(q.rels) == 0 {
		c.writeSelect(b, q, count)
		writeStartFrom(b, q)
	} else {
		b.w("WITH RECURSIVE s0(node_id) AS (SELECT n.id")
		writeStartFrom(b, q)
		b.w(")")

		for i, r := range q.rels {
			writeStep(b, i+1, r)
		}

		b.w(" ")
		c.writeSelect(b, q, count)
		b.w(" FROM (SELECT DISTINCT node_id FROM s", strconv.Itoa(len(q.rels)), ") s CROSS JOIN nodes n ON n.id = s.node_id")
	}

	if !count && (q.limitVal > 0 || q.offsetVal > 0) {
		b.w(" LIMIT ? OFFSET ?")
		limit := q.limitVal
		if limit <= 0 {
			limit = -1
		}
		b.args = append(b.args, limit, max(q.offsetVal, 0))
	}

	c.sql = b.String()
	c.args = b.args
	return c
}

// writeSelect appends the SELECT list: COUNT(*), or n.id followed by the
// columns named by Return. Property paths extract only their JSON value, so
// rows skip decoding the whole properties document.
func (c *compiledQuery) writeSelect(b *sqlBuilder, q *Query, count bool) {
	if count {
		b.w("SELECT COUNT(*)")
		return
	}
	c.cols = allCols
	if len(q.returnCols) > 0 {
		c.cols = nil
		all := slices.Contains(q.returnCols, "properties")
		for _, col := range q.returnCols {
			// Paths are redundant when the whole document is returned.
			if col == "id" || (all && !knownNodeCols[col]) || slices.Contains(c.cols, col) {
				continue
			}
			c.cols = append(c.cols, col)
		}
	}
	if q.withLabels && !slices.Contains(c.cols, "labels") {
		// Clip so the append copies instead of writing into allCols.
		c.cols = append(slices.Clip(c.cols), "labels")
	}

	b.w("SELECT n.id")
	for _, col := range c.cols {
		if col == "labels" {
			b.w(", ", labelsExpr)
			continue
		}
		if knownNodeCols[col] {
			b.w(", n.", col)
			continue
		}
		b.w(", n.properties -> ?")
		b.args = append(b.args, "$."+col)
	}
}

// writeStartFrom appends the FROM and WHERE clauses selecting the start nodes n.
func writeStartFrom(b *sqlBuilder, q *Query) {
	// Without ANALYZE statistics the planner rates the label and name indexes
	// equally and drives from the label, scanning every node carrying it. A
	// name equality is almost always more selective, so the unary + removes
	// the label index from consideration and the label is checked by primary
	// key per matched node.
	label := "nl.label"
	for _, w := range q.wheres {
		if !w.isJSON && w.field == "name" && (w.op == "=" || w.op == "IS") {
			label = "+nl.label"
			break
		}
	}
	b.w(" FROM nodes n JOIN node_labels nl ON nl.node_id = n.id WHERE ", label, " = ?")
	b.args = append(b.args, q.matchLabel)
	writeWheres(b, "n", q.wheres)
}

// writeStep appends CTE s<i>(node_id): the nodes reached from s<i-1> by r.
func writeStep(b *sqlBuilder, i int, r relStep) {
	step := strconv.Itoa(i)
	prev := "s" + strconv.Itoa(i-1)
	reached := "s" + step
	if len(r.wheres) > 0 {
		reached = "u" + step
	}

	switch {
	case r.maxHops == 1:
		b.w(", ", reached, "(node_id) AS (")
		writeHops(b, prev, r, hopAll, "")
		b.w(")")
	case r.breadthFirst:
		writeBFS(b, step, prev, reached, r)
	default:
		// UNION deduplicates on (node_id, depth), which bounds the work per
		// depth level by the node count and stops cycles.
		walk := "w" + step
		b.w(", ", walk, "(node_id, depth) AS (SELECT node_id, 0 FROM ", prev, " UNION ")
		writeHops(b, walk, r, hopRecursive, "p.depth < "+strconv.Itoa(r.maxHops))
		b.w("), ", reached, "(node_id) AS (SELECT node_id FROM ", walk, " WHERE depth >= ", strconv.Itoa(r.minHops), ")")
	}

	if len(r.wheres) > 0 {
		b.w(", s", step, "(node_id) AS (SELECT n.id FROM (SELECT DISTINCT node_id FROM ", reached, ") x CROSS JOIN nodes n ON n.id = x.node_id")
		writeWheres(b, "n", r.wheres)
		b.w(")")
	}
}

// writeBFS appends CTEs reaching nodes within 1..maxHops of prev, expanding
// each node once. The recursive CTE deduplicates on (node, depth), so on
// graphs with cycles it re-expands a node at every depth it is reachable.
// Here frontier f<k> holds nodes first reached at depth k: the expansion of
// f<k-1> minus all earlier frontiers. A start node is not excluded, so it is
// reached again through a cycle, matching the recursive form.
//
// Each level costs a fixed ~20µs to build its temporary tables, so this only
// wins when the walk revisits many nodes; see Query.BreadthFirst.
func writeBFS(b *sqlBuilder, step, prev, reached string, r relStep) {
	front := func(k int) string { return "f" + step + "_" + strconv.Itoa(k) }
	for k := 1; k <= r.maxHops; k++ {
		from := prev
		exclude := ""
		if k > 1 {
			from = front(k - 1)
			var ex strings.Builder
			for j := 1; j < k; j++ {
				if j > 1 {
					ex.WriteString(" UNION ALL ")
				}
				ex.WriteString("SELECT node_id FROM " + front(j))
			}
			exclude = ex.String()
		}
		// MATERIALIZED keeps each frontier from being re-evaluated by every
		// later level that excludes it.
		b.w(", ", front(k), "(node_id) AS MATERIALIZED (")
		writeHops(b, from, r, hopNew, exclude)
		b.w(")")
	}
	b.w(", ", reached, "(node_id) AS (")
	for k := 1; k <= r.maxHops; k++ {
		if k > 1 {
			b.w(" UNION ALL ")
		}
		b.w("SELECT node_id FROM ", front(k))
	}
	b.w(")")
}

// hopMode selects how writeHops shapes its SELECT.
type hopMode int

const (
	// hopAll selects every reached node, duplicates included.
	hopAll hopMode = iota
	// hopRecursive is the recursive member of a walk CTE: it carries
	// depth + 1 and filters on the depth condition.
	hopRecursive
	// hopNew selects distinct reached nodes not in the exclusion subquery.
	hopNew
)

// writeHops appends a SELECT of the nodes one hop from table from along r.
// cond is the depth condition for hopRecursive, or the exclusion subquery for
// hopNew (empty to exclude nothing).
// Both directions compile to two SELECTs, one per endpoint column, so each can
// use its covering index; an OR in the join condition defeats index use.
func writeHops(b *sqlBuilder, from string, r relStep, mode hopMode, cond string) {
	type side struct{ near, far string }
	var sides []side
	switch r.direction {
	case Outgoing:
		sides = []side{{"source_id", "target_id"}}
	case Incoming:
		sides = []side{{"target_id", "source_id"}}
	case Both:
		sides = []side{{"source_id", "target_id"}, {"target_id", "source_id"}}
	}

	for j, s := range sides {
		if j > 0 {
			if mode == hopAll {
				b.w(" UNION ALL ")
			} else {
				b.w(" UNION ")
			}
		}
		b.w("SELECT ")
		if mode == hopNew && len(sides) == 1 {
			b.w("DISTINCT ")
		}
		b.w("e.", s.far)
		if mode == hopRecursive {
			b.w(", p.depth + 1")
		}
		b.w(" FROM ", from, " p CROSS JOIN edges e ON e.", s.near, " = p.node_id AND e.type = ?")
		switch {
		case mode == hopRecursive:
			b.w(" WHERE ", cond)
		case mode == hopNew && cond != "":
			b.w(" WHERE e.", s.far, " NOT IN (", cond, ")")
		}
		b.args = append(b.args, r.edgeType)
	}
}

func writeWheres(b *sqlBuilder, tableAlias string, wheres []whereClause) {
	for _, w := range wheres {
		b.w(" AND ")
		list := w.op == "IN" || w.op == "NOT IN"
		if w.isJSON {
			// The path is bound, never interpolated, so any key is safe.
			sample := w.value
			if list {
				sample = w.elem
			}
			b.w(jsonExtractExpr(tableAlias, sample))
			b.args = append(b.args, "$."+w.field)
		} else {
			// w.field is checked against filterCols.
			b.w(tableAlias, ".", w.field)
		}
		// w.op is checked against validOps.
		if list {
			b.w(" ", w.op, " (SELECT value FROM json_each(?))")
		} else {
			b.w(" ", w.op, " ?")
		}
		b.args = append(b.args, w.value)
	}
}

// jsonExtractExpr returns the SQLite expression that extracts the property at
// a bound path parameter, cast to match the Go type of value.
func jsonExtractExpr(tableAlias string, value any) string {
	extract := tableAlias + ".properties ->> ?"
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return "CAST(" + extract + " AS INTEGER)"
	case float32, float64:
		return "CAST(" + extract + " AS REAL)"
	default:
		return extract
	}
}

func (c *compiledQuery) execute(conn *sqlite.Conn) (*Result, error) {
	res := &Result{index: -1}

	err := sqlitex.Execute(conn, c.sql, &sqlitex.ExecOptions{
		Args: c.args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			n := &Node{ID: stmt.ColumnInt64(0)}
			for i, col := range c.cols {
				i++
				switch col {
				case "name":
					n.Name = stmt.ColumnText(i)
				case "labels":
					if text := stmt.ColumnText(i); text != "[]" {
						if err := json.Unmarshal([]byte(text), &n.Labels); err != nil {
							return fmt.Errorf("graph: decode labels: %w", err)
						}
					}
				case "created_at":
					n.CreatedAt = stmt.ColumnText(i)
				case "updated_at":
					n.UpdatedAt = stmt.ColumnText(i)
				case "properties":
					props, err := UnmarshalProperties(stmt.ColumnText(i))
					if err != nil {
						return err
					}
					n.Properties = props
				default:
					if n.Properties == nil {
						n.Properties = map[string]any{}
					}
					if stmt.ColumnType(i) == sqlite.TypeNull {
						continue
					}
					var v any
					if err := json.Unmarshal([]byte(stmt.ColumnText(i)), &v); err != nil {
						return fmt.Errorf("graph: decode property %q: %w", col, err)
					}
					n.Properties[col] = v
				}
			}
			res.rows = append(res.rows, ResultRow{Node: n})
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("graph: query execution: %w", err)
	}

	return res, nil
}

func (c *compiledQuery) executeCount(conn *sqlite.Conn) (int64, error) {
	var count int64
	err := sqlitex.Execute(conn, c.sql, &sqlitex.ExecOptions{
		Args: c.args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			count = stmt.ColumnInt64(0)
			return nil
		},
	})
	if err != nil {
		return 0, fmt.Errorf("graph: count execution: %w", err)
	}
	return count, nil
}

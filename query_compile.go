package graph

import (
	"fmt"
	"strings"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type compiledQuery struct {
	sql  string
	args []any
}

const nodeCols = "n.id, n.name, n.created_at, n.updated_at, n.properties"

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
	var sb strings.Builder

	if len(q.rels) == 0 {
		if count {
			sb.WriteString("SELECT COUNT(*)")
		} else {
			sb.WriteString("SELECT " + nodeCols)
		}
		c.writeStartFrom(&sb, q)
	} else {
		sb.WriteString("WITH RECURSIVE s0(node_id) AS (SELECT n.id")
		c.writeStartFrom(&sb, q)
		sb.WriteString(")")

		for i, r := range q.rels {
			c.writeStep(&sb, i+1, r)
		}

		if count {
			sb.WriteString(" SELECT COUNT(*)")
		} else {
			sb.WriteString(" SELECT " + nodeCols)
		}
		fmt.Fprintf(&sb, " FROM (SELECT DISTINCT node_id FROM s%d) s CROSS JOIN nodes n ON n.id = s.node_id", len(q.rels))
	}

	if !count && (q.limitVal > 0 || q.offsetVal > 0) {
		sb.WriteString(" LIMIT ? OFFSET ?")
		limit := q.limitVal
		if limit <= 0 {
			limit = -1
		}
		c.args = append(c.args, limit, max(q.offsetVal, 0))
	}

	c.sql = sb.String()
	return c
}

// writeStartFrom appends the FROM and WHERE clauses selecting the start nodes n.
func (c *compiledQuery) writeStartFrom(sb *strings.Builder, q *Query) {
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
	fmt.Fprintf(sb, " FROM nodes n JOIN node_labels nl ON nl.node_id = n.id WHERE %s = ?", label)
	c.args = append(c.args, q.matchLabel)
	c.writeWheres(sb, "n", q.wheres)
}

// writeStep appends CTE s<i>(node_id): the nodes reached from s<i-1> by r.
func (c *compiledQuery) writeStep(sb *strings.Builder, i int, r relStep) {
	prev := fmt.Sprintf("s%d", i-1)
	reached := fmt.Sprintf("s%d", i)
	if len(r.wheres) > 0 {
		reached = fmt.Sprintf("u%d", i)
	}

	if r.maxHops == 1 {
		fmt.Fprintf(sb, ", %s(node_id) AS (", reached)
		c.writeHops(sb, prev, "", r)
		sb.WriteString(")")
	} else {
		// UNION deduplicates on (node_id, depth), which bounds the work per
		// depth level by the node count and stops cycles.
		walk := fmt.Sprintf("w%d", i)
		fmt.Fprintf(sb, ", %s(node_id, depth) AS (SELECT node_id, 0 FROM %s UNION ", walk, prev)
		c.writeHops(sb, walk, fmt.Sprintf("p.depth < %d", r.maxHops), r)
		fmt.Fprintf(sb, "), %s(node_id) AS (SELECT node_id FROM %s WHERE depth >= %d)", reached, walk, r.minHops)
	}

	if len(r.wheres) > 0 {
		fmt.Fprintf(sb, ", s%d(node_id) AS (SELECT n.id FROM (SELECT DISTINCT node_id FROM %s) x CROSS JOIN nodes n ON n.id = x.node_id", i, reached)
		c.writeWheres(sb, "n", r.wheres)
		sb.WriteString(")")
	}
}

// writeHops appends a SELECT of one hop from table from along r. For a
// recursive CTE, the SELECT also carries depth + 1 and the cond filter.
// Both directions compile to two SELECTs joined by UNION ALL (UNION inside a
// recursive CTE) so each side can use its covering index; an OR in the join
// condition defeats index use.
func (c *compiledQuery) writeHops(sb *strings.Builder, from, cond string, r relStep) {
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

	recursive := cond != ""
	for j, s := range sides {
		if j > 0 {
			if recursive {
				sb.WriteString(" UNION ")
			} else {
				sb.WriteString(" UNION ALL ")
			}
		}
		fmt.Fprintf(sb, "SELECT e.%s", s.far)
		if recursive {
			sb.WriteString(", p.depth + 1")
		}
		fmt.Fprintf(sb, " FROM %s p CROSS JOIN edges e ON e.%s = p.node_id AND e.type = ?", from, s.near)
		if recursive {
			sb.WriteString(" WHERE " + cond)
		}
		c.args = append(c.args, r.edgeType)
	}
}

func (c *compiledQuery) writeWheres(sb *strings.Builder, tableAlias string, wheres []whereClause) {
	for _, w := range wheres {
		clause, arg := buildWhereExpr(tableAlias, w)
		sb.WriteString(" AND ")
		sb.WriteString(clause)
		c.args = append(c.args, arg)
	}
}

// buildWhereExpr builds a single WHERE expression clause and returns it with the bound arg.
func buildWhereExpr(tableAlias string, w whereClause) (string, any) {
	if w.isJSON {
		expr := jsonExtractExpr(tableAlias, w.field, w.value)
		return fmt.Sprintf("%s %s ?", expr, w.op), w.value
	}
	return fmt.Sprintf("%s.%s %s ?", tableAlias, w.field, w.op), w.value
}

// jsonExtractExpr returns the SQLite expression to extract and optionally cast a JSON property.
func jsonExtractExpr(tableAlias, path string, value any) string {
	extract := fmt.Sprintf("%s.properties->>'$.%s'", tableAlias, path)
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("CAST(%s AS INTEGER)", extract)
	case float32, float64:
		return fmt.Sprintf("CAST(%s AS REAL)", extract)
	default:
		return extract
	}
}

func (c *compiledQuery) execute(conn *sqlite.Conn) (*Result, error) {
	res := &Result{index: -1}

	err := sqlitex.Execute(conn, c.sql, &sqlitex.ExecOptions{
		Args: c.args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			n := &Node{
				ID:        stmt.ColumnInt64(0),
				Name:      stmt.ColumnText(1),
				CreatedAt: stmt.ColumnText(2),
				UpdatedAt: stmt.ColumnText(3),
			}
			var err error
			n.Properties, err = UnmarshalProperties(stmt.ColumnText(4))
			if err != nil {
				return err
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

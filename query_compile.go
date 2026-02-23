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

// Known node columns that map directly (not JSON).
var knownNodeCols = map[string]bool{
	"id": true, "name": true, "created_at": true, "updated_at": true, "properties": true,
}

func (q *Query) compile() (*compiledQuery, error) {
	c := &compiledQuery{}

	if len(q.rels) == 0 {
		return q.compileSimpleMatch(c)
	}
	return q.compileTraversal(c)
}

func (q *Query) compileCount() (*compiledQuery, error) {
	c := &compiledQuery{}

	if len(q.rels) == 0 {
		return q.compileSimpleMatchCount(c)
	}
	return q.compileTraversalCount(c)
}

// compileSimpleMatch: no Related() steps, just a node label + where filter.
func (q *Query) compileSimpleMatch(c *compiledQuery) (*compiledQuery, error) {
	var sb strings.Builder
	sb.WriteString("SELECT n.id, n.name, n.created_at, n.updated_at, n.properties")
	sb.WriteString(" FROM nodes n")
	sb.WriteString(" JOIN node_labels nl ON nl.node_id = n.id")
	sb.WriteString(" WHERE nl.label = ?")
	c.args = append(c.args, q.matchLabel)

	for _, w := range q.wheres {
		clause, arg := buildWhereExpr("n", w)
		sb.WriteString(" AND ")
		sb.WriteString(clause)
		c.args = append(c.args, arg)
	}

	if q.limitVal > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", q.limitVal))
	}
	if q.offsetVal > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", q.offsetVal))
	}

	c.sql = sb.String()
	return c, nil
}

func (q *Query) compileSimpleMatchCount(c *compiledQuery) (*compiledQuery, error) {
	var sb strings.Builder
	sb.WriteString("SELECT COUNT(*)")
	sb.WriteString(" FROM nodes n")
	sb.WriteString(" JOIN node_labels nl ON nl.node_id = n.id")
	sb.WriteString(" WHERE nl.label = ?")
	c.args = append(c.args, q.matchLabel)

	for _, w := range q.wheres {
		clause, arg := buildWhereExpr("n", w)
		sb.WriteString(" AND ")
		sb.WriteString(clause)
		c.args = append(c.args, arg)
	}

	c.sql = sb.String()
	return c, nil
}

// compileTraversal handles queries with Related() steps.
func (q *Query) compileTraversal(c *compiledQuery) (*compiledQuery, error) {
	needsCTE := false
	for _, r := range q.rels {
		if r.maxHops > 1 {
			needsCTE = true
			break
		}
	}

	if needsCTE {
		return q.compileWithCTEs(c)
	}
	return q.compileSingleHopJoins(c)
}

func (q *Query) compileTraversalCount(c *compiledQuery) (*compiledQuery, error) {
	// Build the same traversal but wrap it in COUNT
	inner, err := q.compileTraversal(c)
	if err != nil {
		return nil, err
	}
	// Replace the SELECT columns with COUNT
	// We wrap the whole thing as a subquery
	inner.sql = fmt.Sprintf("SELECT COUNT(*) FROM (%s)", inner.sql)
	return inner, nil
}

// compileSingleHopJoins: all Related() steps are exactly 1 hop, use simple JOINs.
func (q *Query) compileSingleHopJoins(c *compiledQuery) (*compiledQuery, error) {
	var sb strings.Builder

	// Final node alias
	lastAlias := fmt.Sprintf("n%d", len(q.rels))

	sb.WriteString(fmt.Sprintf("SELECT DISTINCT %s.id, %s.name, %s.created_at, %s.updated_at, %s.properties",
		lastAlias, lastAlias, lastAlias, lastAlias, lastAlias))
	sb.WriteString(" FROM nodes n0")
	sb.WriteString(" JOIN node_labels nl0 ON nl0.node_id = n0.id")

	for i, r := range q.rels {
		eAlias := fmt.Sprintf("e%d", i)
		nAlias := fmt.Sprintf("n%d", i+1)
		prevAlias := fmt.Sprintf("n%d", i)

		switch r.direction {
		case Outgoing:
			sb.WriteString(fmt.Sprintf(" JOIN edges %s ON %s.source_id = %s.id AND %s.type = ?",
				eAlias, eAlias, prevAlias, eAlias))
			sb.WriteString(fmt.Sprintf(" JOIN nodes %s ON %s.id = %s.target_id",
				nAlias, nAlias, eAlias))
		case Incoming:
			sb.WriteString(fmt.Sprintf(" JOIN edges %s ON %s.target_id = %s.id AND %s.type = ?",
				eAlias, eAlias, prevAlias, eAlias))
			sb.WriteString(fmt.Sprintf(" JOIN nodes %s ON %s.id = %s.source_id",
				nAlias, nAlias, eAlias))
		case Both:
			sb.WriteString(fmt.Sprintf(" JOIN edges %s ON (%s.source_id = %s.id OR %s.target_id = %s.id) AND %s.type = ?",
				eAlias, eAlias, prevAlias, eAlias, prevAlias, eAlias))
			sb.WriteString(fmt.Sprintf(" JOIN nodes %s ON %s.id = CASE WHEN %s.source_id = %s.id THEN %s.target_id ELSE %s.source_id END",
				nAlias, nAlias, eAlias, prevAlias, eAlias, eAlias))
		}
		c.args = append(c.args, r.edgeType)
	}

	sb.WriteString(" WHERE nl0.label = ?")
	c.args = append(c.args, q.matchLabel)

	// Append starting node wheres
	for _, w := range q.wheres {
		clause, arg := buildWhereExpr("n0", w)
		sb.WriteString(" AND ")
		sb.WriteString(clause)
		c.args = append(c.args, arg)
	}

	// Append per-step node wheres
	for i, r := range q.rels {
		nAlias := fmt.Sprintf("n%d", i+1)
		for _, w := range r.wheres {
			clause, arg := buildWhereExpr(nAlias, w)
			sb.WriteString(" AND ")
			sb.WriteString(clause)
			c.args = append(c.args, arg)
		}
	}

	if q.limitVal > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", q.limitVal))
	} else {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", DefaultMaxResults))
	}
	if q.offsetVal > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", q.offsetVal))
	}

	c.sql = sb.String()
	return c, nil
}

// compileWithCTEs: at least one Related() step has maxHops > 1.
func (q *Query) compileWithCTEs(c *compiledQuery) (*compiledQuery, error) {
	var sb strings.Builder

	sb.WriteString("WITH RECURSIVE ")

	for i, r := range q.rels {
		if i > 0 {
			sb.WriteString(", ")
		}

		stepName := fmt.Sprintf("step%d", i)

		if r.maxHops == 1 {
			// Non-recursive CTE for single-hop steps
			q.writeSingleHopCTE(&sb, c, i, stepName, r)
		} else {
			// Recursive CTE for multi-hop steps
			q.writeRecursiveCTE(&sb, c, i, stepName, r)
		}
	}

	// Final SELECT
	lastStep := fmt.Sprintf("step%d", len(q.rels)-1)
	lastRel := q.rels[len(q.rels)-1]

	sb.WriteString(" SELECT DISTINCT n.id, n.name, n.created_at, n.updated_at, n.properties")
	sb.WriteString(fmt.Sprintf(" FROM %s s", lastStep))
	sb.WriteString(" JOIN nodes n ON n.id = s.node_id")

	if lastRel.maxHops > 1 {
		sb.WriteString(fmt.Sprintf(" WHERE s.depth >= %d AND s.depth <= %d", lastRel.minHops, lastRel.maxHops))
	}

	// Apply wheres on the final step's reached nodes
	for _, w := range lastRel.wheres {
		clause, arg := buildWhereExpr("n", w)
		sb.WriteString(" AND ")
		sb.WriteString(clause)
		c.args = append(c.args, arg)
	}

	if q.limitVal > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", q.limitVal))
	} else {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", DefaultMaxResults))
	}
	if q.offsetVal > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", q.offsetVal))
	}

	c.sql = sb.String()
	return c, nil
}

func (q *Query) writeSingleHopCTE(sb *strings.Builder, c *compiledQuery, stepIdx int, stepName string, r relStep) {
	sb.WriteString(fmt.Sprintf("%s(node_id) AS (", stepName))

	if stepIdx == 0 {
		// First step: source is the MATCH node
		switch r.direction {
		case Outgoing:
			sb.WriteString("SELECT e.target_id FROM nodes n")
			sb.WriteString(" JOIN node_labels nl ON nl.node_id = n.id")
			sb.WriteString(" JOIN edges e ON e.source_id = n.id AND e.type = ?")
		case Incoming:
			sb.WriteString("SELECT e.source_id FROM nodes n")
			sb.WriteString(" JOIN node_labels nl ON nl.node_id = n.id")
			sb.WriteString(" JOIN edges e ON e.target_id = n.id AND e.type = ?")
		case Both:
			sb.WriteString("SELECT CASE WHEN e.source_id = n.id THEN e.target_id ELSE e.source_id END FROM nodes n")
			sb.WriteString(" JOIN node_labels nl ON nl.node_id = n.id")
			sb.WriteString(" JOIN edges e ON (e.source_id = n.id OR e.target_id = n.id) AND e.type = ?")
		}
		sb.WriteString(" WHERE nl.label = ?")
		c.args = append(c.args, r.edgeType, q.matchLabel)

		for _, w := range q.wheres {
			clause, arg := buildWhereExpr("n", w)
			sb.WriteString(" AND ")
			sb.WriteString(clause)
			c.args = append(c.args, arg)
		}
	} else {
		// Subsequent step: source is the previous step's result
		prevStep := fmt.Sprintf("step%d", stepIdx-1)
		prevRel := q.rels[stepIdx-1]

		switch r.direction {
		case Outgoing:
			sb.WriteString(fmt.Sprintf("SELECT e.target_id FROM %s p", prevStep))
			sb.WriteString(" JOIN edges e ON e.source_id = p.node_id AND e.type = ?")
		case Incoming:
			sb.WriteString(fmt.Sprintf("SELECT e.source_id FROM %s p", prevStep))
			sb.WriteString(" JOIN edges e ON e.target_id = p.node_id AND e.type = ?")
		case Both:
			sb.WriteString(fmt.Sprintf("SELECT CASE WHEN e.source_id = p.node_id THEN e.target_id ELSE e.source_id END FROM %s p", prevStep))
			sb.WriteString(" JOIN edges e ON (e.source_id = p.node_id OR e.target_id = p.node_id) AND e.type = ?")
		}
		c.args = append(c.args, r.edgeType)

		if prevRel.maxHops > 1 {
			sb.WriteString(fmt.Sprintf(" WHERE p.depth >= %d AND p.depth <= %d", prevRel.minHops, prevRel.maxHops))
		}
	}

	sb.WriteString(")")
}

func (q *Query) writeRecursiveCTE(sb *strings.Builder, c *compiledQuery, stepIdx int, stepName string, r relStep) {
	sb.WriteString(fmt.Sprintf("%s(node_id, depth) AS (", stepName))

	// Base case
	if stepIdx == 0 {
		// First step: base is the MATCH nodes at depth 0
		sb.WriteString("SELECT n.id, 0 FROM nodes n")
		sb.WriteString(" JOIN node_labels nl ON nl.node_id = n.id")
		sb.WriteString(" WHERE nl.label = ?")
		c.args = append(c.args, q.matchLabel)

		for _, w := range q.wheres {
			clause, arg := buildWhereExpr("n", w)
			sb.WriteString(" AND ")
			sb.WriteString(clause)
			c.args = append(c.args, arg)
		}
	} else {
		// Subsequent step: base is previous step's results
		prevStep := fmt.Sprintf("step%d", stepIdx-1)
		prevRel := q.rels[stepIdx-1]

		sb.WriteString(fmt.Sprintf("SELECT p.node_id, 0 FROM %s p", prevStep))
		if prevRel.maxHops > 1 {
			sb.WriteString(fmt.Sprintf(" WHERE p.depth >= %d AND p.depth <= %d", prevRel.minHops, prevRel.maxHops))
		}
	}

	sb.WriteString(" UNION ")

	// Recursive case
	switch r.direction {
	case Outgoing:
		sb.WriteString(fmt.Sprintf("SELECT e.target_id, t.depth + 1 FROM %s t", stepName))
		sb.WriteString(" JOIN edges e ON e.source_id = t.node_id")
	case Incoming:
		sb.WriteString(fmt.Sprintf("SELECT e.source_id, t.depth + 1 FROM %s t", stepName))
		sb.WriteString(" JOIN edges e ON e.target_id = t.node_id")
	case Both:
		sb.WriteString(fmt.Sprintf("SELECT CASE WHEN e.source_id = t.node_id THEN e.target_id ELSE e.source_id END, t.depth + 1 FROM %s t", stepName))
		sb.WriteString(" JOIN edges e ON (e.source_id = t.node_id OR e.target_id = t.node_id)")
	}
	sb.WriteString(fmt.Sprintf(" WHERE e.type = ? AND t.depth < %d", r.maxHops))
	c.args = append(c.args, r.edgeType)

	sb.WriteString(")")
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

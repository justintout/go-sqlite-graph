package graph

// Result holds the results of a query execution.
type Result struct {
	rows  []ResultRow
	index int // current iteration position, starts at -1
}

// ResultRow holds a single result row.
type ResultRow struct {
	Node *Node
}

// Next advances to the next result row. Returns false when exhausted.
func (r *Result) Next() bool {
	r.index++
	return r.index < len(r.rows)
}

// Row returns the current result row.
func (r *Result) Row() ResultRow {
	return r.rows[r.index]
}

// All returns all result rows as a slice.
func (r *Result) All() []ResultRow {
	return r.rows
}

// Nodes returns all result nodes.
func (r *Result) Nodes() []*Node {
	nodes := make([]*Node, len(r.rows))
	for i, row := range r.rows {
		nodes[i] = row.Node
	}
	return nodes
}

// Len returns the number of result rows.
func (r *Result) Len() int {
	return len(r.rows)
}

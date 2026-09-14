package graph

import "zombiezen.com/go/sqlite/sqlitemigration"

const migration1 = `
CREATE TABLE nodes (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    properties TEXT    NOT NULL DEFAULT '{}'
);

CREATE TABLE node_labels (
    node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    label   TEXT    NOT NULL,
    PRIMARY KEY (node_id, label)
);

CREATE TABLE edges (
    id         INTEGER PRIMARY KEY,
    source_id  INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_id  INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    type       TEXT    NOT NULL,
    name       TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    properties TEXT    NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_node_labels_label ON node_labels(label);
CREATE INDEX idx_node_labels_node_id ON node_labels(node_id);
CREATE INDEX idx_edges_source ON edges(source_id);
CREATE INDEX idx_edges_target ON edges(target_id);
CREATE INDEX idx_edges_type ON edges(type);
CREATE INDEX idx_edges_source_type ON edges(source_id, type);
CREATE INDEX idx_edges_target_type ON edges(target_id, type);
`

// migration2 reworks indexes for traversal and write cost.
//
// A single-column index on edges(type) is a trap: without ANALYZE statistics
// the planner prefers it for "source_id = ? AND type = ?" joins, scanning every
// edge of that type per expanded node. The composite indexes append the far
// endpoint so traversal steps never touch the edges table. Prefix-redundant
// indexes are dropped to cut write cost, and node_labels becomes WITHOUT ROWID
// so its primary key is the table instead of a second b-tree.
const migration2 = `
DROP INDEX idx_edges_source;
DROP INDEX idx_edges_target;
DROP INDEX idx_edges_type;
DROP INDEX idx_edges_source_type;
DROP INDEX idx_edges_target_type;
CREATE INDEX idx_edges_source_type_target ON edges(source_id, type, target_id);
CREATE INDEX idx_edges_target_type_source ON edges(target_id, type, source_id);

CREATE TABLE node_labels_new (
    node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    label   TEXT    NOT NULL,
    PRIMARY KEY (node_id, label)
) WITHOUT ROWID;
INSERT INTO node_labels_new (node_id, label) SELECT node_id, label FROM node_labels;
DROP TABLE node_labels;
ALTER TABLE node_labels_new RENAME TO node_labels;
CREATE INDEX idx_node_labels_label ON node_labels(label);

CREATE INDEX idx_nodes_name ON nodes(name);
`

func graphSchema() sqlitemigration.Schema {
	return sqlitemigration.Schema{
		Migrations: []string{migration1, migration2},
	}
}

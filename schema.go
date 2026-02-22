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

func graphSchema() sqlitemigration.Schema {
	return sqlitemigration.Schema{
		Migrations: []string{migration1},
	}
}

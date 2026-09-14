# Comparison benchmarks

Benchmarks go-sqlite-graph against [GraphQLite](https://github.com/colliery-io/graphqlite), a SQLite extension with Cypher support. This is a separate module because it needs CGo (`mattn/go-sqlite3`) to load the extension.

Each graph is loaded into three engines, and the harness fails if they disagree on any result size:

| Engine | What runs |
|---|---|
| `ours` | The go-sqlite-graph API on modernc SQLite (pure Go) |
| `ours-sql-on-c` | The SQL go-sqlite-graph generates, run on C SQLite against the same database file |
| `graphqlite` | Variable-length Cypher through `cypher_rows`, with data loaded the way GraphQLite's bulk insert API loads it |

`ours-sql-on-c` separates the library's schema and query design from the cost of the pure-Go SQLite engine.

Graphs:

- `random-10k`: 10k nodes, 3 uniform random outgoing edges per node.
- `powerlaw-100k`: 100k nodes, 500k edges with Zipf-distributed targets. Node 0 is a hub with ~10k in-edges.

Every query starts from a node matched by label and name, and returns the distinct names of reached nodes.

## Running

Get the GraphQLite extension. The PyPI wheel ships a prebuilt library:

```
pip download graphqlite --no-deps --only-binary=:all: -d /tmp/gql
unzip -o /tmp/gql/*.whl -d /tmp/gql
```

Then run:

```
GRAPHQLITE_EXT=/tmp/gql/graphqlite/graphqlite.dylib go test -bench . -benchmem
```

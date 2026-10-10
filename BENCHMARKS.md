# Benchmarks

```
go test -bench=. -benchmem .
```

Results on an Apple M3 Max, in-memory database unless noted:

```
BenchmarkTraversalChain/hops=1                           7.6µs    2220 B/op     33 allocs/op  (100-node chain)
BenchmarkTraversalChain/hops=10                         31.4µs    8889 B/op    173 allocs/op

BenchmarkTraversalFanout/depth=2                        33.1µs   10417 B/op    201 allocs/op  (13 nodes)
BenchmarkTraversalFanout/depth=4                       199.0µs   90999 B/op   1824 allocs/op  (121 nodes)
BenchmarkTraversalFanout/depth=6                        1.92ms  826277 B/op  16408 allocs/op  (1093 nodes)

BenchmarkTraversalDense/hops=1                          16.2µs    5263 B/op    101 allocs/op  (500 nodes, 5 edges/node)
BenchmarkTraversalDense/hops=3                         279.0µs  102656 B/op   2196 allocs/op
BenchmarkTraversalDense/hops=10                         9.72ms  373273 B/op   7985 allocs/op

BenchmarkTraversalLarge/out/hops=1                      13.0µs    3738 B/op     68 allocs/op  (10k nodes, 3 edges/node)
BenchmarkTraversalLarge/out/hops=3                      98.8µs   30919 B/op    648 allocs/op
BenchmarkTraversalLarge/out/hops=6                      2.55ms  757988 B/op  16267 allocs/op
BenchmarkTraversalLarge/both/hops=1                     23.6µs    6007 B/op    118 allocs/op
BenchmarkTraversalLarge/both/hops=3                    484.2µs  164892 B/op   3498 allocs/op

BenchmarkReturn/all                                     6.48ms 2936649 B/op  62979 allocs/op  (6 hops over 10k nodes, 10 properties each)
BenchmarkReturn/name                                    1.77ms  122085 B/op   2059 allocs/op
BenchmarkReturn/name+2props                             4.21ms  952593 B/op  16273 allocs/op

BenchmarkBreadthFirst/chain/hops=10/default             41.1µs    9049 B/op    174 allocs/op
BenchmarkBreadthFirst/chain/hops=10/breadth-first      250.2µs   25037 B/op    224 allocs/op
BenchmarkBreadthFirst/dense/hops=10/default             9.47ms  373405 B/op   7985 allocs/op
BenchmarkBreadthFirst/dense/hops=10/breadth-first       2.07ms  389410 B/op   8035 allocs/op
BenchmarkBreadthFirst/large/both/hops=4/default         2.28ms  258041 B/op   6800 allocs/op
BenchmarkBreadthFirst/large/both/hops=4/breadth-first   1.76ms  262176 B/op   6816 allocs/op

BenchmarkMatchSimple/nodes=100                           4.1µs    1619 B/op     22 allocs/op
BenchmarkMatchSimple/nodes=10000                         4.4µs    1619 B/op     22 allocs/op

BenchmarkCreateNode                                     11.6µs    1679 B/op     36 allocs/op
BenchmarkCreateNodeFile                                 67.5µs    1686 B/op     35 allocs/op  (on disk, WAL)
BenchmarkCreateEdge                                      7.2µs     714 B/op     17 allocs/op
BenchmarkBulkInsertTx/batch=1000                        6.47ms  835897 B/op  29476 allocs/op
```

- Traversal cost follows the number of nodes reached, not the graph size. One hop costs about the same on a 100-node chain and on a 10k-node graph.
- Dense graphs cost the most, because the default walk expands a node at every depth it is reachable. `BreadthFirst` cuts 10 hops on the dense graph from 9.5ms to 2.1ms, and raises 10 hops on a chain from 41µs to 250µs.
- Decoding each node's JSON properties is a large share of a big traversal. Returning only `name` cuts a 6-hop traversal from 6.5ms to 1.8ms.
- A match by name uses an index, so its cost does not grow with the number of nodes.

[`bench/compare`](bench/compare) compares this library with [GraphQLite](https://github.com/colliery-io/graphqlite) on a 10k-node random graph and a 100k-node power-law graph.

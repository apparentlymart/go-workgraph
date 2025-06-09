// Package workgraph provides some low-level utilities for coordinating
// a number of workers that are all collaborating to produce different parts
// of some overall result, with dynamically-discovered dependencies between
// those workers.
//
// Workers and requests form a bipartite graph. Every request has exactly one
// worker responsible for resolving it, and every worker is waiting for zero or
// one requests to be resolved. If worker A attempts to wait for a result that
// will be produced by worker B which is waiting for another result that A is
// responsible for then all requests in that chain immediately fail to avoid
// deadlock.
//
// This is a "nuts-and-bolts" abstraction intended to be used as an
// implementation detail of a higher-level system, and is not intended to be
// treated as a cross-cutting concern that appears in another library's exported
// API. Use idomatic Go features like blocking calls or channels to represent
// relationships between concurrent work in larger scopes.
package workgraph

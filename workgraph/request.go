package workgraph

import (
	"iter"
	"runtime"
)

// NewRequest begins a new request and returns both its resolver and its
// promise.
//
// The given worker is initially responsible for resolving the request.
func NewRequest[T any](responsibleWorker *Worker) (Resolver[T], Promise[T]) {
	newInner := newRequestInner(responsibleWorker.inner)

	resolver := Resolver[T]{
		inner: newInner,
	}
	consumer := Promise[T]{
		inner: newInner,
	}

	// The following ensures that the Worker can't get garbage collected during
	// the statements above, in case the caller has given us its last remaining
	// pointer to this Worker. If this _is_ the last remaining pointer then
	// the new result might become resolved as failed immediately after this
	// statement, before we even return.
	runtime.KeepAlive(responsibleWorker)
	return resolver, consumer
}

// ResolverContainer is implemented by types that in some sense "contain" [Resolver]
// objects, allowing the responsibility for all of those results to be passed
// in aggregate to a new task when calling [NewWorker].
//
// [*Resolver] itself implements this interface, so callers with no need for any
// higher-level aggregation can pass individual [Resolver] values directly instead
// of implementing this interface themselves.
type ResolverContainer interface {
	ContainedResolvers() iter.Seq[AnyResolver]
}

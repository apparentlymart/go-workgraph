package workgraph

import (
	"iter"
	"runtime"
)

// NewPendingResult returns the resolver and consumer "ends" of a newly-created
// result slot, with the given worker initially responsible for resolving it.
func NewPendingResult[T any](responsibleWorker *Worker) (ResultResolver[T], ResultConsumer[T]) {
	newInner := newResultInner(responsibleWorker.inner)

	resolver := ResultResolver[T]{
		inner: newInner,
	}
	consumer := ResultConsumer[T]{
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

// ResultContainer is implemented by types that in some sense "contain" [ResultResolver]
// objects, allowing the responsibility for all of those results to be passed
// in aggregate to a new task when calling [NewWorker].
//
// [*ResultResolver] itself implements this interface, so callers with no need for any
// higher-level aggregation can pass individual [ResultResolver] values directly instead
// of implementing this interface themselves.
type ResultContainer interface {
	ContainedResultResolvers() iter.Seq[AnyResultResolver]
}

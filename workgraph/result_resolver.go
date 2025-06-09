package workgraph

import (
	"iter"
)

type ResultResolver[T any] struct {
	inner *resultInner
}

var _ ResultContainer = ResultResolver[int]{}

func (r ResultResolver[T]) Resolve(resolvingWorker *Worker, val T, err error) {
	r.inner.resolveExplicit(resolvingWorker, val, err)
}

func (r ResultResolver[T]) ResolveSuccess(resolvingWorker *Worker, val T) {
	r.Resolve(resolvingWorker, val, nil)
}

func (r ResultResolver[T]) ResolveError(resolvingWorker *Worker, err error) {
	var zero T
	r.Resolve(resolvingWorker, zero, err)
}

func (r ResultResolver[T]) ResultID() ResultID {
	return r.inner.ResultID()
}

// ContainedResults implements [ResultContainer], reporting the reciever itself
// as the only result in the container.
func (r ResultResolver[T]) ContainedResultResolvers() iter.Seq[AnyResultResolver] {
	return func(yield func(AnyResultResolver) bool) {
		yield(r)
	}
}

// resultInner implements AnyResultResolver.
func (r ResultResolver[T]) resultInner() *resultInner {
	return r.inner
}

// AnyResultResolver is an interface implemented by all instantiations of
// the generic type [ResultResolver], regardless of their result type.
//
// This is used along with [ResultContainer] to delegate resolvers from one
// worker to another, where it doesn't matter what specific type each resolver
// has.
type AnyResultResolver interface {
	resultInner() *resultInner
}

var _ AnyResultResolver = ResultResolver[int]{}

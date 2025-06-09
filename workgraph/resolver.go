package workgraph

import (
	"iter"
)

// A Resolver is used by the [Worker] that is responsible for resolving a
// request to report its result, thereby unblocking any other workers that
// are waiting for its resolution.
type Resolver[T any] struct {
	inner *requestInner
}

var _ ResolverContainer = Resolver[int]{}

// Report resolves the request with both a result value and an error, both
// of which will be returned from any [Promise.Await] calls for the
// associated request.
func (r Resolver[T]) Report(resolvingWorker *Worker, val T, err error) {
	r.inner.resolveExplicit(resolvingWorker, val, err)
}

// ReportSuccess is a helper for [Resolver.Report] which automatically sets
// the error to nil, suggesting a successful result.
func (r Resolver[T]) ReportSuccess(resolvingWorker *Worker, val T) {
	r.Report(resolvingWorker, val, nil)
}

// ReportSuccess is a helper for [Resolver.Report] which automatically sets
// the value part of the result to the zero value of T, suggesting an error
// result without any useful accompanying error.
func (r Resolver[T]) ReportError(resolvingWorker *Worker, err error) {
	var zero T
	r.Report(resolvingWorker, zero, err)
}

// RequestID returns a unique identifier for the request that this resolver
// belongs to.
//
// This can be compared with [RequestID] values in errors returned by this
// library in situations that would otherwise cause a deadlock.
func (r Resolver[T]) RequestID() RequestID {
	return r.inner.ResultID()
}

// ContainedResolvers implements [ResolverContainer], reporting the reciever
// itself as the only resolver in the container.
func (r Resolver[T]) ContainedResolvers() iter.Seq[AnyResolver] {
	return func(yield func(AnyResolver) bool) {
		yield(r)
	}
}

// resultInner implements AnyResultResolver.
func (r Resolver[T]) resultInner() *requestInner {
	return r.inner
}

// AnyResolver is an interface implemented by all instantiations of
// the generic type [Resolver], regardless of their result type.
//
// This is used along with [ResolverContainer] to delegate resolvers from one
// worker to another, where it doesn't matter what value type each resolver
// has.
type AnyResolver interface {
	resultInner() *requestInner
}

var _ AnyResolver = Resolver[int]{}

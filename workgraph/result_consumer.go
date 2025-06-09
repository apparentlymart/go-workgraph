package workgraph

import "fmt"

// ResultConsumer is a handle through which many different workers can wait
// for a result to become available.
type ResultConsumer[T any] struct {
	inner *resultInner
}

// Await blocks until the associated result has been resolved, or until
// a problem forces it to resolve with a usage error to avoid deadlocking.
func (rc ResultConsumer[T]) Await(requestingWorker *Worker) (T, error) {
	if waitingFor := requestingWorker.inner.awaiting.Load(); waitingFor != nil {
		// Each worker can be awaiting only one result at a time, so this
		// is always a bug in the caller.
		panic(fmt.Sprintf("worker %p awaits multiple results", requestingWorker.inner))
	}
	if resolution := rc.inner.resolution.Load(); resolution != nil {
		// If the result was already resolved then we'll return as quickly
		// as possible to minimize overhead.
		return resolutionRet[T](resolution)
	}

	// If we get here then we need to do the slow-path await.
	resolution := rc.inner.await(requestingWorker)
	return resolutionRet[T](resolution)
}

package workgraph

import (
	"fmt"
)

// Promise is a handle through which many different workers can wait
// for the result of a request to become available.
type Promise[T any] struct {
	inner *requestInner
}

// Await blocks until the associated request has been resolved, or until
// a problem forces it to resolve with a usage error to avoid deadlocking.
func (rc Promise[T]) Await(requestingWorker *Worker) (T, error) {
	if waitingFor := requestingWorker.inner.awaiting.Load(); waitingFor != nil {
		// Each worker can be awaiting only one promise at a time, so this
		// is always a bug in the caller.
		panic(fmt.Sprintf("worker %p awaits multiple promises", requestingWorker.inner))
	}
	if result := rc.inner.result.Load(); result != nil {
		// If the request was already resolved then we'll return as quickly
		// as possible to minimize overhead.
		return resultRet[T](result)
	}

	// If we get here then we need to do the slow-path await.
	result := rc.inner.await(requestingWorker)
	return resultRet[T](result)
}

func (rc Promise[T]) isNil() bool {
	return rc.inner == nil
}

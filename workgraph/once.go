package workgraph

import (
	"sync"
)

// Once is similar in principle to the standard library's [sync.Once], but
// implemented in terms of the workgraph concepts so that it can provide
// similar guarantees such as detecting when a Once execution ends up
// indirectly depending on its own result.
type Once[T any] struct {
	mu      sync.Mutex
	promise Promise[T]
	req_id  RequestID
}

// Do calls the function f if and only if Do is being called for the first
// time on this instance of [Once].
//
// Given a specific instance of [Once], only the first call with invoke the
// given f, even if f has a different value on each call.
//
// forWorker is the handle for the worker that the result is requested on
// behalf of. The given function is called with its own separate [Worker]
// that is responsible for providing the return value. If f directly or
// indirectly causes another call to Do on the same Once then all affected
// calls will fail with [ErrSelfDependency].
func (o *Once[T]) Do(forWorker *Worker, f func(*Worker) (T, error)) (T, error) {
	o.mu.Lock()
	if o.promise.isNil() {
		// This is the first call, so we'll establish the inner request
		// and start executing the function in a separate goroutine.
		resolver, promise := NewRequest[T](forWorker)
		o.promise = promise
		o.req_id = resolver.RequestID()
		WithNewAsyncWorker(func(w *Worker) {
			ret, err := f(w)
			resolver.Report(w, ret, err)
		}, resolver)
	} else {
	}
	o.mu.Unlock()

	return o.promise.Await(forWorker)
}

// RequestID returns the identifier of the internal request that represents
// the completion of all calls to [Once.Do] on this object.
//
// If [Once.Do] has not yet been called, this returns [NoRequest] to indicate
// that no request has started yet.
func (o *Once[T]) RequestID() RequestID {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.req_id
}

// OnceFunc returns a function that, when called for the first time, will
// run f using a newly-created [Worker], and then that and all subsequent
// calls will return whatever that function returns.
//
// This is a convenience wrapper for [Once] in situations where the
// underlying [RequestID] is unimportant and so just a function pointer
// is sufficient, and where it's helpful to capture the function to
// run inside the result so it can be called from many different locations.
func OnceFunc[T any](f func(*Worker) (T, error)) func(*Worker) (T, error) {
	var once Once[T]
	return func(requestingWorker *Worker) (T, error) {
		return once.Do(requestingWorker, f)
	}
}

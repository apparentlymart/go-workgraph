package workgraph

// ErrUnresolved is returned by [Promise.Await] if the [Worker]
// responsible for resolving the request is garbage-collected before the
// request is resolved.
//
// This suggests a bug in the implementation of the responsible worker, since
// it should ensure that all requests it is responsible for are either resolved
// or delegated to another worker before its [Worker] object goes out of scope.
type ErrUnresolved struct {
	// RequestID is the request that was unresolved. This is always the ID of
	// the request whose [Promise] the Await method was called on.
	RequestID RequestID
}

func (err ErrUnresolved) Error() string {
	return "responsible worker was dropped before request was resolved"
}

// ErrSelfDependency is returned by [Promise.Await] if a direct or
// indirect self-dependency is created in the worker-and-request graph by
// this or some other call to [Promise.Await].
//
// All Await calls blocking on any result in the detected dependency cycle
// immediately fail with this error.
type ErrSelfDependency struct {
	// RequestIDs are the identifiers of the requests included in the
	// dependency cycle. Callers may use this in conjunction with their own
	// records of the meaning of each request ID to return a higher-level error
	// that describes the set of requested operations that together caused the
	// problem.
	RequestIDs []RequestID
}

func (err ErrSelfDependency) Error() string {
	return "self-dependency detected"
}

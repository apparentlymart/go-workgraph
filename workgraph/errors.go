package workgraph

// ErrUnresolved is returned by [ResultConsumer.Await] if the [Worker]
// responsible for resolving the result is garbage collected before the
// result is resolved.
//
// This suggests a bug in the implementation of the responsible worker, since
// it should ensure that all results it is responsible for are either resolved
// or delegated to another worker before its [Worker] object goes out of scope.
type ErrUnresolved struct {
	// ResultID is the result that was unresolved. This is always the ID of
	// the result whose [ResultConsumer] the Await method was called on.
	ResultID ResultID
}

func (err ErrUnresolved) Error() string {
	return "responsible worker was dropped before result was resolved"
}

// ErrSelfDependency is returned by [ResultConsumer.Await] if a direct or
// indirect self-dependency is created in the worker-to-result graph by
// this or some other call to [ResultConsumer.Await].
//
// All Await calls blocking on any result in the detected dependency cycle
// immediately fail with this error.
type ErrSelfDependency struct {
	// ResultIDs are the identifiers of the results included in the
	// dependency cycle. Callers may use this in conjunction with their own
	// records of the meaning of each result ID to return a higher-level error
	// that describes the set of requested operations that together caused the
	// problem.
	ResultIDs []ResultID
}

func (err ErrSelfDependency) Error() string {
	return "self-dependency detected"
}

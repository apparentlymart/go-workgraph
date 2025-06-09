package workgraph

import (
	"runtime"
)

// A Worker represents a specific linear codepath that will ultimately resolve
// zero or more results.
//
// It's ultimately up to the caller to decide what exactly "linear codepath"
// means. The simplest mental model is for each Worker to belong to one
// goroutine and for that worker to go out of scope once the goroutine exits,
// with no other goroutine interacting with it.
//
// However, the only hard constraint is that each worker can only be waiting on
// zero or one results at a time, and so two goroutines can potentially share
// a single worker as long as they somehow arrange for at most one of them
// to interact with the worker at a time.
//
// A [Worker] object must be kept live (i.e. not garbage collected) until it
// has either provided or delegated all of the results that it's responsible
// for, or else those results will all fail with an error. This is a best-effort
// mechanism to reclaim other workers that could otherwise be blocked
// indefinitely, but should not be relied on in any "happy path" because the
// Go garbage collection details are intentionally underspecified to allow
// for future improvements.
type Worker struct {
	// We separate the object held by the external caller from the object we
	// use internally so that this outer object can be garbage collected once
	// the caller is finished with it while allowing the inner object to
	// also be referred to by other objects.
	//
	// [NewWorker] uses a cleanup function associated with the Worker pointer
	// it returns to notify the inner object once the outer object has been
	// collected.
	inner *workerInner
}

// NewWorker allocates a new [Worker], optionally transferring responsibility
// for resolving some results.
//
// Callers are responsible for ensuring that a caller passing results to this
// function was previously considered to be responsible for those results.
// Although there are no immediate checks that the caller was already
// responsible for the given results (the relationship between codepaths and
// workers is the caller's concern), incorrect use of this can potentially be
// detected later if the previous responsible worker subsequently attempts to
// resolve
func NewWorker(delegatedResults ...ResultContainer) *Worker {
	// The new "inner" is initially not awaiting any result.
	newInner := newWorkerInner()

	// We can safely transfer responsibility for all of the given result
	// objects here without any self-dependency checking, because the new
	// worker is initially not waiting for any results itself and so it
	// cannot possibly participate in a self-dependency cycle.
	for _, container := range delegatedResults {
		for result := range container.ContainedResultResolvers() {
			inner := result.resultInner()
			inner.setResponsibleWorker(newInner)
		}
	}

	ret := &Worker{
		inner: newInner,
	}
	// The object we return has a cleanup function that notifies its associated
	// inner once it gets collected, so we can force-unblock anything that's
	// waiting on any results this result was responsible for.
	runtime.AddCleanup(ret, (*workerInner).handleDropped, newInner)
	return ret
}

func WithNewAsyncWorker(f func(*Worker), delegatedResults ...ResultContainer) {
	worker := NewWorker(delegatedResults...)
	go f(worker)
}

package workgraph

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"weak"
)

// requestInner is the internal part of a request that is shared between its
// resolver and its promises.
//
// This inner part intentionally has the compile-time-chosen result type
// erased, to allow [AnyResolver] to be implemented by all instantiations
// of the generic [Resolver] type.
type requestInner struct {
	// responsible is the primary representation of the directed graph edge
	// between a request and the worker that's currently responsible for
	// resolving it. This is never nil but it can change over time as
	// responsibility is delegated between workers.
	//
	// This is an atomic pointer so we can perform the first pass of
	// self-dependency checking without acquiring any locks.
	responsible atomic.Pointer[workerInner]

	mu     sync.Mutex
	cond   *sync.Cond
	result atomic.Pointer[requestResult]
}

func (ri *requestInner) ResultID() RequestID {
	return RequestID{
		ptr: weak.Make(ri),
	}
}

func (ri *requestInner) await(requestingWorker *Worker) *requestResult {
	// This function deals with the "slow-path" await, after
	// [Promise.Await] dealt with some fast-path situations. However,
	// we haven't been holding any locks so far and so we'll need to recheck
	// some things in case the situation has changed due to the actions of
	// another concurrent goroutine.
	//
	// The overall idea of this is based on the ideas in
	// "An Ownership Policy and Deadlock Detector for Promises" by Caleb Voss
	// and Vivek Sarkar at Georgia Institute of Technology, arXiv:2101.01312v1.
	// The following is essentially the logic from their "Algorithm 2" ported
	// to Go. We use atomic memory accesses to avoid acquring broadly-scoped
	// locks that would likely cause contention between workers.

	swapped := requestingWorker.inner.awaiting.CompareAndSwap(nil, ri)
	if !swapped {
		// Apparently another goroutine has begun waiting with this worker
		// in the meantime since [Promise.Await] did its initial check.
		panic(fmt.Sprintf("worker %p awaits multiple promises", requestingWorker.inner))
	}
	defer func() {
		// Before we return we need to set "awaiting" back to nil again to
		// let the requesting worker await other promises, but since we might
		// return before we aquire locks we could again be racing with another
		// goroutine trying to use the same worker object, so we'll detect
		// that here.
		// (This corresponds to the "try/finally" pseudocode in at the
		// end of the algorithm from the paper, since Go does not have
		// exceptions.)
		swappedBack := requestingWorker.inner.awaiting.CompareAndSwap(ri, nil)
		if !swappedBack {
			panic(fmt.Sprintf("worker %p awaits multiple promises", requestingWorker.inner))
		}
	}()

	// Before we begin waiting we'll check whether our change to the "awaiting"
	// field above has caused a cycle in the worker-result graph. Because each
	// worker awaits zero or one results and each result has exactly one
	// responsible worker we can check this using only a linear walk along those
	// edges.
	selfDependency, _ := detectSelfDependency(ri, requestingWorker.inner, false)
	if selfDependency {
		// We've found a self-dependency but we want to be able to report
		// which requests were affected by it and so we'll repeat the same
		// work again but this time collect up all of the request nodes we
		// encounter along the way. This redundancy allows us to avoid
		// allocating the request slice on the happy path. We could potentially
		// get a slightly different result this time but nonetheless we'll
		// still be reporting at least some of the results that were affected
		// by the cycle.
		_, failedResults := detectSelfDependency(ri, requestingWorker.inner, true)
		resultIDs := make([]RequestID, 0, len(failedResults))
		for _, result := range failedResults {
			resultIDs = append(resultIDs, result.ResultID())
		}
		err := ErrSelfDependency{RequestIDs: resultIDs}
		for _, result := range failedResults {
			result.resolveUsageFault(err)
		}
		// Note that we've now resolved "ri" as a side-effect of the above,
		// since it will always be one of the failed results. Therefore we
		// can fall through here and detect below that the result is now
		// resolved.
	}

	// We'll now finally actually aquire the lock, since we know it's now
	// safe for us to block without causing a deadlock.
	ri.mu.Lock()
	for {
		if resolution := ri.result.Load(); resolution != nil {
			ri.mu.Unlock()
			return resolution
		}
		ri.cond.Wait() // ri.mu is automatically unlocked while waiting, and then relocked before this returns
	}
}

// detectSelfDependency is the main loop for self-dependency detection in
// [requestInner.await], factored out so that we can run it a second time in
// a more expensive mode (with collectFailedReqs set) to collect context when
// we're going to report an error.
func detectSelfDependency(currentReq *requestInner, requestingWorker *workerInner, collectFailedReqs bool) (bool, []*requestInner) {
	// We populate this only if collectFailedReqs is true. Otherwise we
	// just ignore it to avoid allocating.
	var failedReqs []*requestInner

	currentWorker := currentReq.responsible.Load()
	if collectFailedReqs {
		failedReqs = append(failedReqs, currentReq)
	}
	for currentWorker != requestingWorker {
		if currentReq == nil {
			break
		}
		nextReq := currentWorker.awaiting.Load()
		if nextReq == nil {
			break
		}
		if currentReq.responsible.Load() != currentWorker {
			break
		}
		currentReq = nextReq
		currentWorker = currentReq.responsible.Load()
		if collectFailedReqs {
			failedReqs = append(failedReqs, currentReq)
		}
	}

	// If we've ended up back where we started then we've detected self-dependency.
	return currentWorker == requestingWorker, failedReqs
}

// resolveExplicit is the main resolution function for an "explicit" result,
// meaning that the result is being provided by the worker that's responsible
// for doing so.
func (ri *requestInner) resolveExplicit(resolvingWorker *Worker, val any, err error) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	if got, want := resolvingWorker.inner, ri.responsible.Load(); got != want {
		panic(fmt.Sprintf("request was resolved by worker %p, but %p was responsible", got, want))
	}
	if resolution := ri.result.Load(); resolution != nil {
		// This is already resolved. If it was resolved with a usage error then
		// we'll just silently ignore this call to avoid changing the
		// previously-reported outcome, but if the previous resolution was also
		// explicit then that suggests a bug in the caller and so we'll panic.
		if resolution.IsExplicit() {
			panic("request resolved multiple times")
		}
		return
	}

	ri.result.Store(newExplicitResult(val, err))
	ri.cond.Broadcast()

	// We'll make sure that Worker can't get collected until we're ready to
	// return just to avoid any oddities that might arise if we have the
	// last remaining pointer to this Worker object. (If we let it become dead
	// too soon then all of the results it's responsible for -- presumably
	// including this one -- could get force-resolved with [ErrUnresolved].
	runtime.KeepAlive(resolvingWorker)
}

// resolveUsageFault is a variant resolution function for situations where we
// force an errored resolution from inside this library to report that the
// library has been used incorrectly.
func (ri *requestInner) resolveUsageFault(err error) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	if result := ri.result.Load(); result != nil {
		// This is already resolved, so we'll leave the existing resolution
		// in place because some consumers might already have observed the
		// previous resolution.
		return
	}
	ri.result.Store(newUsageFaultResult(err))
	ri.cond.Broadcast()
}

func newRequestInner(responsibleWorker *workerInner) *requestInner {
	ret := &requestInner{}
	ret.cond = sync.NewCond(&ret.mu)
	ret.setResponsibleWorker(responsibleWorker)
	return ret
}

func (ri *requestInner) setResponsibleWorker(new *workerInner) {
	ri.responsible.Store(new)
}

type requestResult struct {
	value any
	err   error
}

func newExplicitResult(value any, err error) *requestResult {
	if value == nil {
		// Should not be possible because we should always get here through
		// a generic function that enforces value always being a valid value
		// of the result type. (Even if the result type is something that
		// can be nil, the interface value containing it would not be nil.)
		panic("explicit resolution with nil value")
	}
	return &requestResult{
		value: value,
		err:   err,
	}
}

func newUsageFaultResult(err error) *requestResult {
	return &requestResult{
		value: nil, // indicates a usage fault resolution
		err:   err,
	}
}

func (rr *requestResult) IsExplicit() bool {
	// Explicit resolutions always have a non-nil value, even though what's
	// stored in the interface might be a typed nil pointer itself.
	return rr.value != nil
}

func resultRet[T any](result *requestResult) (T, error) {
	// The type assertion below should fail only if value is nil to
	// represent a usage error, in which case we'll just return the
	// zero value of T.
	// (Even if T is a type that can be "nil" itself, a non-usage
	// error will always be saved as a non-nil interface which
	// might contain a nil value of T.)
	value, _ := result.value.(T)
	return value, result.err
}

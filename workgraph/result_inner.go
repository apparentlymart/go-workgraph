package workgraph

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"weak"
)

// resultInner is the internal part of a result that is shared between its
// resolver and its consumers.
//
// This inner part intentionally has the compile-time-chosen result type
// erased, to allow [AnyResultResolver] to be implemented by all instantiations
// of the generic [ResultResolver] type.
type resultInner struct {
	// responsible is the primary representation of the directed graph edge
	// between a result and the worker that's currently responsible for
	// resolving it. This is never nil but it can change over time as
	// responsibility is delegated between workers.
	//
	// This is an atomic pointer so we can perform the first pass of
	// self-dependency checking without acquiring any locks.
	responsible atomic.Pointer[workerInner]

	mu         sync.Mutex
	cond       *sync.Cond
	resolution atomic.Pointer[resultResolution]
}

func (ri *resultInner) ResultID() ResultID {
	return ResultID{
		ptr: weak.Make(ri),
	}
}

func (ri *resultInner) await(requestingWorker *Worker) *resultResolution {
	// This function deals with the "slow-path" await, after
	// [ResultConsumer.Await] dealt with some fast-path situations. However,
	// we haven't been holding any locks so far and so we'll need to recheck
	// some things in case the situation has changed due to the actions of
	// another concurrent goroutine.
	//
	// The overall idea of this is based on the ideas in
	// "An Ownership Policy and Deadlock Detector for Promises" by Caleb Voss
	// and Vivek Sarkar at Georgia Institute of Technology, arXiv:2101.01312v1.
	// The following is essentially the logic from their "Algorithm 2".

	swapped := requestingWorker.inner.awaiting.CompareAndSwap(nil, ri)
	if !swapped {
		// Apparently another goroutine has begun waiting with this worker
		// in the meantime since [ResultConsumer.Await] did its initial check.
		panic(fmt.Sprintf("worker %p awaits multiple results", requestingWorker.inner))
	}
	defer func() {
		// Before we return we need to set "awaiting" back to nil again to
		// let the requesting worker await other results, but since we might
		// return before we aquire locks we could again be racing with another
		// goroutine trying to use the same worker object, so we'll detect
		// that here.
		// (This corresponds to the "try/finally" pseudocode
		swappedBack := requestingWorker.inner.awaiting.CompareAndSwap(ri, nil)
		if !swappedBack {
			panic(fmt.Sprintf("worker %p awaits multiple results", requestingWorker.inner))
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
		// which results were affected by it and so we'll repeat the same
		// work again but this time collect up all of the result nodes we
		// encounter along the way. This redundancy allows us to avoid
		// allocating the result slice on the happy path. We could potentially
		// get a slightly different result this time but nonetheless we'll
		// still be reporting at least some of the results that were affected
		// by the cycle.
		_, failedResults := detectSelfDependency(ri, requestingWorker.inner, true)
		resultIDs := make([]ResultID, 0, len(failedResults))
		for _, result := range failedResults {
			resultIDs = append(resultIDs, result.ResultID())
		}
		err := ErrSelfDependency{ResultIDs: resultIDs}
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
		if resolution := ri.resolution.Load(); resolution != nil {
			ri.mu.Unlock()
			return resolution
		}
		ri.cond.Wait() // ri.mu is automatically unlocked while waiting, and then relocked before this returns
	}
}

func detectSelfDependency(currentResult *resultInner, requestingWorker *workerInner, collectFailedResults bool) (bool, []*resultInner) {
	// We populate this only if collectFailedResults is true. Otherwise we
	// just ignore it to avoid allocating.
	var failedResults []*resultInner

	currentWorker := currentResult.responsible.Load()
	if collectFailedResults {
		failedResults = append(failedResults, currentResult)
	}
	for currentWorker != requestingWorker {
		if currentResult == nil {
			break
		}
		nextResult := currentWorker.awaiting.Load()
		if nextResult == nil {
			break
		}
		if currentResult.responsible.Load() != currentWorker {
			break
		}
		currentResult = nextResult
		currentWorker = currentResult.responsible.Load()
		if collectFailedResults {
			failedResults = append(failedResults, currentResult)
		}
	}

	// If we've ended up back where we started then we've detected self-dependency.
	return currentWorker == requestingWorker, failedResults
}

func (ri *resultInner) resolveExplicit(resolvingWorker *Worker, val any, err error) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	if got, want := resolvingWorker.inner, ri.responsible.Load(); got != want {
		panic(fmt.Sprintf("result was resolved by worker %p, but %p was responsible", got, want))
	}
	if resolution := ri.resolution.Load(); resolution != nil {
		// This is already resolved. If it was resolved with a usage error then
		// we'll just silently ignore this call to avoid changing the
		// previously-reported outcome, but if the previous resolution was also
		// explicit then that suggests a bug in the caller and so we'll panic.
		if resolution.IsExplicit() {
			panic("result resolved multiple times")
		}
		return
	}

	ri.resolution.Store(newExplicitResolution(val, err))
	ri.cond.Broadcast()

	// We'll make sure that Worker can't get collected until we're ready to
	// return just to avoid any oddities that might arise if we have the
	// last remaining pointer to this Worker object. (If we let it become dead
	// too soon then all of the results it's responsible for -- presumably
	// including this one -- could get force-resolved with [ErrUnresolved].
	runtime.KeepAlive(resolvingWorker)
}

func (ri *resultInner) resolveUsageFault(err error) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	if resolution := ri.resolution.Load(); resolution != nil {
		// This is already resolved, so we'll leave the existing resolution
		// in place because some consumers might already have observed the
		// previous resolution.
		return
	}
	ri.resolution.Store(newUsageFaultResolution(err))
	ri.cond.Broadcast()
}

func newResultInner(responsibleWorker *workerInner) *resultInner {
	ret := &resultInner{}
	ret.cond = sync.NewCond(&ret.mu)
	ret.setResponsibleWorker(responsibleWorker)
	return ret
}

func (ri *resultInner) setResponsibleWorker(new *workerInner) {
	ri.responsible.Store(new)
}

type resultResolution struct {
	value any
	err   error
}

func newExplicitResolution(value any, err error) *resultResolution {
	if value == nil {
		// Should not be possible because we should always get here through
		// a generic function that enforces value always being a valid value
		// of the result type. (Even if the result type is something that
		// can be nil, the interface value containing it would not be nil.)
		panic("explicit resolution with nil value")
	}
	return &resultResolution{
		value: value,
		err:   err,
	}
}

func newUsageFaultResolution(err error) *resultResolution {
	return &resultResolution{
		value: nil, // indicates a usage fault resolution
		err:   err,
	}
}

func (rr *resultResolution) IsExplicit() bool {
	// Explicit resolutions always have a non-nil value, even though what's
	// stored in the interface might be a typed nil pointer itself.
	return rr.value != nil
}

func resolutionRet[T any](resolution *resultResolution) (T, error) {
	// The type assertion below should fail only if value is nil to
	// represent a usage error, in which case we'll just return the
	// zero value of T.
	// (Even if T is a type that can be "nil" itself, a non-usage
	// error will always be saved as a non-nil interface which
	// might contain a nil value of T.)
	value, _ := resolution.value.(T)
	return value, resolution.err
}

package workgraph

import (
	"sync/atomic"
)

// workerInner is the real representation of a worker, which participates
// in the worker/request graph.
//
// The exported representation [Worker] is separated so that the only pointers
// to it are from outside of this package and we can use its finalizer as
// a signal that the results that the worker is responsible for can never
// be provided.
type workerInner struct {
	// awaiting is the primary representation of the directed graph edge
	// between a worker and the result it's currently awaiting, if any.
	//
	// This is an atomic pointer so we can perform the first pass of
	// self-dependency checking without acquiring any locks.
	awaiting atomic.Pointer[requestInner]
}

func newWorkerInner() *workerInner {
	return &workerInner{}
}

func (wi *workerInner) handleDropped() {

}

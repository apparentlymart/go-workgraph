package workgraph_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/google/go-cmp/cmp"
)

func TestHappyPath(t *testing.T) {
	mainWorker := workgraph.NewWorker()
	greetingResolver, greetingPromise := workgraph.NewRequest[string](mainWorker)
	greeteeResolver, greeteePromise := workgraph.NewRequest[string](mainWorker)
	workgraph.WithNewAsyncWorker(func(w *workgraph.Worker) {
		greetingResolver.ReportSuccess(w, "Hello")
	}, greetingResolver)
	workgraph.WithNewAsyncWorker(func(w *workgraph.Worker) {
		// This nested worker is unnecessary and just here to make this test
		// a little more interesting.
		workgraph.WithNewAsyncWorker(func(w *workgraph.Worker) {
			greeteeResolver.ReportSuccess(w, "world")
		}, greeteeResolver)
	}, greeteeResolver)

	// mainWorker is now allowed to await both promises because it has
	// delegated their resolution to the other workers.
	greeting, err := greetingPromise.Await(mainWorker)
	if err != nil {
		t.Errorf("unexpected error awaiting greeting: %s", err)
	}
	greetee, err := greeteePromise.Await(mainWorker)
	if err != nil {
		t.Errorf("unexpected error awaiting greetee: %s", err)
	}

	gotMessage := fmt.Sprintf("%s, %s!", greeting, greetee)
	wantMessage := "Hello, world!"
	if gotMessage != wantMessage {
		t.Errorf("unexpected result\ngot:  %s\nwant: %s", gotMessage, wantMessage)
	}
}

func TestSelfDependencyDirect(t *testing.T) {
	mainWorker := workgraph.NewWorker()
	resolver, promise := workgraph.NewRequest[string](mainWorker)
	value, err := promise.Await(mainWorker)
	if err == nil {
		t.Fatalf("unexpected success with value %#v; want self-dependency error", value)
	}
	selfDepErr, ok := err.(workgraph.ErrSelfDependency)
	if !ok {
		t.Fatalf("wrong error type %T; want %T", err, selfDepErr)
	}
	wantResultIDs := []workgraph.RequestID{resolver.RequestID()}
	if diff := cmp.Diff(wantResultIDs, selfDepErr.RequestIDs); diff != "" {
		t.Error("wrong request ids\n" + diff)
	}
}

func TestSelfDependencyIndirect(t *testing.T) {
	mainWorker := workgraph.NewWorker()
	resolver1, promise1 := workgraph.NewRequest[string](mainWorker)
	resolver2, promise2 := workgraph.NewRequest[string](mainWorker)
	workgraph.WithNewAsyncWorker(func(w *workgraph.Worker) {
		val, err := promise2.Await(w)
		resolver1.Report(w, val, err)
	}, resolver1)
	workgraph.WithNewAsyncWorker(func(w *workgraph.Worker) {
		val, err := promise1.Await(w)
		resolver2.Report(w, val, err)
	}, resolver2)

	value, err := promise1.Await(mainWorker)
	if err == nil {
		t.Fatalf("unexpected success with value %#v; want self-dependency error", value)
	}
	selfDepErr, ok := err.(workgraph.ErrSelfDependency)
	if !ok {
		t.Fatalf("wrong error type %T; want %T", err, selfDepErr)
	}
	t.Logf("affected request ids: %#v", selfDepErr.RequestIDs)

	// The reported ResultIDs are not guaranteed to be any particular order
	// but we expect both to be present.
	if got, want := len(selfDepErr.RequestIDs), 2; got != want {
		t.Fatalf("wrong number of failed request ids %d; want %d", got, want)
	}
	if !slices.Contains(selfDepErr.RequestIDs, resolver1.RequestID()) {
		t.Errorf("resolver1's ResultID is not mentioned in the error")
	}
	if !slices.Contains(selfDepErr.RequestIDs, resolver2.RequestID()) {
		t.Errorf("resolver2's ResultID is not mentioned in the error")
	}
}

func TestUnresolved(t *testing.T) {
	t.Skip("not implemented yet")

	// This particular test is a little tricky because it's relying on behavior
	// of the Go runtime's garbage collector that is not technically guaranteed:
	// it's possible that the cleanup associated with a worker object may run
	// long after it becomes unreachable, or may even never run at all.
	//
	// If this appears to fail after upgrading to a new version of Go then a
	// good place to start with debugging is to see if the GC behavior has
	// changed in that new version.
	mainWorker := workgraph.NewWorker()
	resolver, promise := workgraph.NewRequest[string](mainWorker)
	workgraph.WithNewAsyncWorker(func(w *workgraph.Worker) {
		// We intentionally don't resolve the promise here, instead
		// just letting our worker go out of scope and thus hopefully
		// get garbage collected and cause the waiter below to return
		// an error instead of deadlocking.
	}, resolver)

	value, err := promise.Await(mainWorker)
	if err == nil {
		t.Fatalf("unexpected success with value %#v; want self-dependency error", value)
	}
	unresolvedErr, ok := err.(workgraph.ErrUnresolved)
	if !ok {
		t.Fatalf("wrong error type %T; want %T", err, unresolvedErr)
	}
	if unresolvedErr.RequestID != resolver.RequestID() {
		t.Errorf("error has the wrong RequestID")
	}
}

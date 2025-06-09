package workgraph_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/google/go-cmp/cmp"
)

func TestOnce_happy(t *testing.T) {
	var once workgraph.Once[string]

	getGreeting := func(forWorker *workgraph.Worker) (string, error) {
		return once.Do(forWorker, func(w *workgraph.Worker) (string, error) {
			return "Hello, world!", nil
		})
	}

	type Result struct {
		Value string
		Err   error
	}

	var wg sync.WaitGroup
	wg.Add(2)
	var result1, result2 Result
	workgraph.WithNewAsyncWorker(func(w *workgraph.Worker) {
		value, err := getGreeting(w)
		result1 = Result{
			Value: value,
			Err:   err,
		}
		wg.Done()
	})
	workgraph.WithNewAsyncWorker(func(w *workgraph.Worker) {
		value, err := getGreeting(w)
		result2 = Result{
			Value: value,
			Err:   err,
		}
		wg.Done()
	})
	wg.Wait()

	wantResult := Result{
		Value: "Hello, world!",
		Err:   nil,
	}
	if diff := cmp.Diff(wantResult, result1); diff != "" {
		t.Error("wrong result1\n" + diff)
	}
	if diff := cmp.Diff(wantResult, result2); diff != "" {
		t.Error("wrong result2\n" + diff)
	}
}

func TestOnce_selfDependencyDirect(t *testing.T) {
	var once workgraph.Once[string]
	_, err := once.Do(workgraph.NewWorker(), func(w *workgraph.Worker) (string, error) {
		return once.Do(w, func(w *workgraph.Worker) (string, error) {
			// We should not get here because only the first call to once.Do
			// actually runs the given function.
			panic("inner function was called")
		})
	})
	if err == nil {
		t.Fatal("unexpected success; want error")
	}
	selfDepErr, ok := err.(workgraph.ErrSelfDependency)
	if !ok {
		t.Fatalf("wrong error type %T; want %T", err, selfDepErr)
	}
	wantResultIDs := []workgraph.RequestID{once.RequestID()}
	if diff := cmp.Diff(wantResultIDs, selfDepErr.RequestIDs); diff != "" {
		t.Error("wrong request ids\n" + diff)
	}
}

func TestOnceFunc(t *testing.T) {
	var calls atomic.Int32
	getResult := workgraph.OnceFunc(func(w *workgraph.Worker) (string, error) {
		calls.Add(1)
		return "hello", fmt.Errorf("expected error")
	})

	result1, err1 := getResult(workgraph.NewWorker())
	if got, want := result1, "hello"; got != want {
		t.Errorf("wrong result1\ngot:  %s\nwant: %s", got, want)
	}
	if got, want := err1.Error(), "expected error"; got != want {
		t.Errorf("wrong result1\ngot:  %s\nwant: %s", got, want)
	}
	result2, err2 := getResult(workgraph.NewWorker())
	if got, want := result2, "hello"; got != want {
		t.Errorf("wrong result1\ngot:  %s\nwant: %s", got, want)
	}
	if got, want := err2.Error(), "expected error"; got != want {
		t.Errorf("wrong result1\ngot:  %s\nwant: %s", got, want)
	}

	if got, want := calls.Load(), int32(1); got != want {
		t.Errorf("wrong number of calls %d; want %d", got, want)
	}
}

func TestOnceFunc_selfDependencyDirect(t *testing.T) {
	// The following arranges for the getResult function to call itself
	// immediately, which should be detected as a self-dependency rather
	// than as infinite recursion.
	var f func(w *workgraph.Worker) (string, error)
	var getResult func(*workgraph.Worker) (string, error)
	f = func(w *workgraph.Worker) (string, error) {
		return getResult(w)
	}
	getResult = workgraph.OnceFunc(f)

	_, err := getResult(workgraph.NewWorker())
	if err == nil {
		t.Fatal("unexpected success; want error")
	}
	selfDepErr, ok := err.(workgraph.ErrSelfDependency)
	if !ok {
		t.Fatalf("wrong error type %T; want %T", err, selfDepErr)
	}
	// We can't test the exact RequestID in this case because the
	// internal RequestID in OnceFunc isn't exposed, but we can test
	// that we have the expected number.
	if got, want := len(selfDepErr.RequestIDs), 1; got != want {
		t.Errorf("wrong number of request ids %d; want %d", got, want)
	}
}

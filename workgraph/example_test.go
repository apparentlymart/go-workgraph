package workgraph_test

import (
	"fmt"
	"log"

	"github.com/apparentlymart/go-workgraph/workgraph"
)

// This contrived example just returns static strings to keep things simple,
// but in more typical use these "getters" could perform more complex
// operations such as fetching data from elsewhere, blocking for as long
// as they want as long as they can only block on each other through
// workgraph-based features.
func ExampleOnceFunc() {
	// A more realistic usage would involve these "getters" being constructed
	// in different parts of the program and then composed together, but
	// this is all written inline just to avoid distracting with unrelated
	// complexity.
	greetingGetter := workgraph.OnceFunc(func(w *workgraph.Worker) (string, error) {
		return "Hello", nil
	})
	nameGetter := workgraph.OnceFunc(func(w *workgraph.Worker) (string, error) {
		return "world", nil
	})
	msgGetter := workgraph.OnceFunc(func(w *workgraph.Worker) (string, error) {
		// The call to greetingGetter blocks until the greeting is ready.
		greeting, err := greetingGetter(w)
		if err != nil {
			return "", fmt.Errorf("greeting failed: %w", err)
		}
		// The call to nameGetter blocks until the name is ready.
		name, err := nameGetter(w)
		if err != nil {
			return "", fmt.Errorf("name failed: %w", err)
		}
		return fmt.Sprintf("%s, %s!", greeting, name), nil
	})

	// We need an initial worker to represent the main goroutine.
	w := workgraph.NewWorker()

	// This call to msgGetter blocks until the full message is ready, which
	// indirectly blocks on greetingGetter and nameGetter's results.
	msg, err := msgGetter(w)
	if err != nil {
		log.Fatalf("message failed: %s", err)
	}
	fmt.Println(msg)
	// Output: Hello, world!
}

func ExampleErrSelfDependency() {
	// This example uses [OnceFunc] just for simplicity's sake, but this
	// general idea applies to any interaction between workers where a
	// worker tries to block on a result it's currently responsible for either
	// directly or indirectly.
	var a, b func(*workgraph.Worker) (string, error)
	a = workgraph.OnceFunc(func(w *workgraph.Worker) (string, error) {
		return b(w)
	})
	b = workgraph.OnceFunc(func(w *workgraph.Worker) (string, error) {
		return a(w)
	})

	// Initial worker for the main goroutine
	w := workgraph.NewWorker()

	result, err := a(w)
	switch err := err.(type) {
	case nil:
		fmt.Printf("result is %q\n", result)
	case workgraph.ErrSelfDependency:
		fmt.Printf("self-dependency error involving %d requests", len(err.RequestIDs))
		// NOTE: If you actually want to make use of the requset IDs then you'd
		// need to either work with [workgraph.Resolver] objects directly or
		// use [workgraph.Once] in order to have request ids to compare with
		// those in the error message.
	default:
		fmt.Printf("unexpected error: %s\n", err)
	}

	// Output: self-dependency error involving 2 requests
}

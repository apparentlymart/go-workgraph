// Package workgraph provides some low-level utilities for coordinating
// a number of workers that are all collaborating to produce different parts
// of some overall result, with dynamically-discovered dependencies between
// those workers.
//
// Workers and requests form a bipartite graph. Every request has exactly one
// worker responsible for resolving it, and every worker is waiting for zero or
// one requests to be resolved. If worker A attempts to wait for a result that
// will be produced by worker B which is waiting for another result that A is
// responsible for then all requests in that chain immediately fail to avoid
// deadlock.
//
// This is a "nuts-and-bolts" abstraction intended to be used as an
// implementation detail of a higher-level system, and is not intended to be
// treated as a cross-cutting concern that appears in another library's exported
// API. Use idomatic Go features like blocking calls or channels to represent
// relationships between concurrent work in larger scopes.
//
// # The Workgraph Rules
//
// The Go programming language (intentionally) does not have anything resembling
// "goroutine-local storage", and so workgraph must unfortunately perform its
// own tracking of the relationships between goroutines in order to detect
// self-reference problems and return an error instead of deadlocking. This
// requires cooperation with the calling program to produce an accurate graph of
// the current work in progress, and so there are some rules that any caller
// must follow.
//
// The most important rule is that each goroutine that interacts with the
// workgraph API must use exactly one [Worker] object to do so. Using the
// same worker object across multiple goroutines or using multiple workers in
// the same goroutine will cause workgraph's view of the world to be
// inconsistent with the Go runtime's view of the world, which unfortunately
// means that deadlocks and data races can become possible.
//
// Another important rule is that two goroutines that are using workgraph
// must communicate with one another using workgraph promises exclusively. If
// one goroutine blocks on another in any way other than calling [Promise.Await]
// then workgraph will be unaware of that relationship and so again deadlocks
// can become possible.
//
// At any instant each active [Promise] has exactly one [Worker] that is
// responsible for resolving it using its [Resolver]. The initially-responsible
// worker is the one passed to [NewRequest]. If a promise is not resolved before
// its responsible worker is garbage-collected then an await on the promise
// returns [ErrUnresolved]. For this mechanism to work it's important that each
// [Worker] object become eligible for garbage collection once its associated
// goroutine exits. The worker responsible for a promise can transfer that
// responsibility to another worker by passing the associated resolver as an
// argument to [NewWorker]. Only the currently-responsible worker for a promise
// is allowed to resolve it.
//
// At any instant each [Worker] is awaiting the result of either zero or one
// [Promise]. "Awaiting" means making a blocking call to [Promise.Await].
// Whenever a worker is about to begin waiting, workgraph checks to make sure
// that this would not cause a cycle in the bipartite graph of promises and
// workers. Awaiting fails with [ErrSelfDependency] if such a cycle is detected.
//
// # Usage Tips
//
// The previous section described the rules that callers must follow for this
// library to work as intended. This section has some practical tips for how
// to structure code so that it's harder to accidentally violate those rules.
//
// First, consider using [Once] or [OnceFunc] if they are appropriate for your
// situation. Although using those does not completely avoid the need for
// callers to take care of the rules, it does encapsulate the problem of
// deciding which worker is responsible for resolution -- whichever one happens
// to request the result first -- so the calling workers don't need to be
// directly aware of each other or coordinate responsibilty themselves.
//
// Consider using [WithNewAsyncWorker] to start any new goroutines that will
// interact with workgraph. This utility really just calls [NewWorker]
// and passes its result to the given function on a new goroutine, but that
// means that the lifetime of the [Worker] object is closely matched to the
// goroutine as long as a pointer to the worker never escapes to the heap.
//
// Regardless of whether or not you use [WithNewAsyncWorker], store pointers to
// [Worker] objects only on the stack. In other words: only in local variables,
// function arguments, and return values. This makes sure that a worker object
// becomes eligible for garbage collection no later than the associated
// goroutine exits. If you find you do need to store a pointer to a worker on
// the heap, such as when capturing within the closure for a function pointer
// that escapes, then ensure that the outer object referring to that allocation
// is only reachable from the stack.
//
// Try to structure your code so that only code in the goroutine associated
// with the worker responsible for a promise can reach that promise's resolver.
// For example, if you transfer responsibility to a new worker when calling
// [NewWorker], consider letting the variable that previously contained the
// resolver go out of scope or assign the zero value of [Resolver] to it to
// reinforce that access through that variable after that point is incorrect.
//
// If you're intending to use the request ids from [ErrSelfDependency] errors
// to produce end-user-friendly error messages describing what participated
// in a self-dependency cycle, make sure that each end-user-relevant work
// item correlates to one [NewRequest] call and that each one is the
// responsibility of a separate [Worker], because that will then cause
// workgraph's internal graph of the work in progress to match the end-user
// model of execution as closely as possible.
//
// If you build your own abstractions that wrap promises in similar ways as
// [Once] does, consider exposing the underlying [RequestID] for each
// encapsulated promise as an aid to error handling and debugging. Unless your
// wrapping abstraction encapsulates the decision about who is responsible for
// resolving a promise (as [Once] does), consider also having your type
// implement [ResolverContainer] so that callers can pass it directly to
// [NewWorker] to delegate responsibility without having to unwrap and then
// re-wrap it.
//
// Overall, try to keep your usage of workgraph scoped to as small a part of
// your program as possible and minimize how it's exposed in your public API.
// It's much harder to ensure that the overall program is correctly following
// the workgraph rules when the uses are distributed widely across a program.
// If multiple subsystems in your program need to collaborate using workgraph,
// try to build a wrapping abstraction that's better tailored to your specific
// use-case and which encapsulates the workgraph rules in terms of its own
// rules that are more natural for the overall shape of your program.
package workgraph

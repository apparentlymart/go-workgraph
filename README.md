# `workgraph` for Go

This is a relatively low-level utility library to help with situations where
an overall result needs to be built from a number of interdependent goroutines
whose final dependencies might not be discovered until runtime.

The introduces a few new concepts to help with modelling such situations:

- "Requests" are the conceptual model for some work that might depend on
  the outcomes of other requests being dealt with concurrently in other
  goroutines.
- A "worker" represents some sequential code that will, during its work,
  wait for the outcome of zero or more requests, and will report the results
  of zero or more other requests.

  Workers interact with the following two additional concepts, which represent
  their two different roles in interacting with requests:

  - A "resolver" is used by the one worker that is responsible for reporting the
    result of a request once it's complete.
  - A "promise" is used by zero or more other workers that need to make use
    of the request result once it has been reported.

The inclusion of "worker" in this model allows for two special behaviors that
are helpful when the dependencies between requests are decided dynamically
based on user input:

- If two workers attempt to depend on each other's work, both requests will
  immediately fail with an error to prevent a deadlock.
- If the worker responsible for resolving a request fails to do so before it
  is garbage collected, all waiters are eventually unblocked with an error to
  avoid them being blocked indefinitely.

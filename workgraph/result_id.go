package workgraph

import (
	"fmt"
	"weak"
)

// ResultID represents an opaque but comparable unique identifier for a
// [ResultResolver], which may or may not still be live.
//
// ResultID values are used in some error types returned by this package when
// reporting situations that could cause deadlock. Callers can therefore
// maintain a lookup table from ResultID to some higher-level representation
// of the meaning of a result to allow including more relevant context in
// externally-facing error results.
type ResultID struct {
	// We use a weak pointer here because we only care about pointer identity
	// and not about the pointee itself. Internally this creates an extra
	// indirection through a heap-allocated pointer value where the pointer
	// to that allocation is actually what we're comparing when using a
	// ResultID as a comparable identifier, whereas the underlying resultInner
	// remains eligible for garbage collection.
	ptr weak.Pointer[resultInner]
}

// Equal returns true if other is the same [ResultID] as the receiver.
//
// This is equivalent to using the "==" operator to compare two values, but
// is implemented here to work better with libraries like Google's "go-cmp"
// which try to perform deep comparison when no Equal method is present.
func (rid ResultID) Equal(other ResultID) bool {
	return rid == other
}

// String returns a human-oriented string representation of the result ID.
//
// This is intended for debug messages only. Do not use the result as a unique
// key for a [ResultID]; this type is comparable so it can act as its own
// unique key.
func (rid ResultID) String() string {
	return fmt.Sprintf("%p", rid.ptr.Value())
}

func (rid ResultID) GoString() string {
	return fmt.Sprintf("workgraph.ResultID(%s)", rid.String())
}

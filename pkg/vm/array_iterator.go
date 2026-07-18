package vm

// ArrayIterState is the shared mutable state behind the built-in array
// iterator's `next` method. The builtins package hangs it on the
// NativeFunctionObject of the per-iterator next closure; the closure and the
// OpArrayIterNext fast path in the dispatch loop both read and advance the
// same Index, so mixing manual it.next() calls with a for-of over the same
// iterator stays coherent.
//
// The for-of fast path is sound because the compiler caches the `next` method
// in a register once per loop (matching the spec's IteratorRecord.[[NextMethod]]
// caching): OpIterFastCheck inspects that cached value once, and a replaced or
// user-defined next simply fails the check and takes the generic call path.
type ArrayIterState struct {
	Arr   *ArrayObject
	Index int
}

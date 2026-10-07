package vm

import "unsafe"

// InstantiateChunk returns a copy of a compiled chunk that can run on a VM
// without sharing mutable state with the original or with any other copy. It
// is what lets a program be compiled once and then run in many VMs or realms:
// the immutable parts (bytecode, line/column tables, exception table, string
// and number constants, global-layout metadata) are shared, while everything a
// run mutates is fresh - the per-site inline caches, and every FunctionObject
// in the constant pool, which are runtime objects (lazily created .prototype
// and property bags, [[HomeObject]], the realm they were created in, a cached
// closure) rather than plain compile output.
//
// The chunk passed in must never have been run itself: instantiation copies
// FunctionObjects as they are, which is only the compile-time state if nothing
// has executed them. Callers keep a pristine chunk and run only instances.
//
// ok is false if a constant has a kind this function does not know how to give
// a run its own copy of (an enum's backing object, for instance, which the
// program mutates at run time). The caller must then fall back to compiling
// from source rather than risk sharing state between runs.
func InstantiateChunk(c *Chunk) (inst *Chunk, ok bool) {
	if c == nil {
		return nil, false
	}
	n := *c // shares Code, Lines, Columns, ExceptionTable, global metadata, ...
	n.propInlineCaches = nil
	// Compile-time dedup tables; nothing adds constants once compilation ends.
	n.stringConstCache, n.intConstCache, n.floatConstCache = nil, nil, nil

	n.Constants = make([]Value, len(c.Constants))
	for i, k := range c.Constants {
		switch k.Type() {
		case TypeUndefined, TypeNull, TypeBoolean, TypeIntegerNumber, TypeFloatNumber, TypeString, TypeBigInt:
			n.Constants[i] = k
		case TypeFunction:
			fn := *k.AsFunction()
			child, childOK := InstantiateChunk(fn.Chunk)
			if !childOK {
				return nil, false
			}
			fn.Chunk = child
			fn.Properties = nil
			fn.cachedClosure = nil
			n.Constants[i] = Value{typ: TypeFunction, obj: unsafe.Pointer(&fn)}
		default:
			return nil, false
		}
	}
	return &n, true
}

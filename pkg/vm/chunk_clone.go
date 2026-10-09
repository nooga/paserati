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
		case TypeUndefined, TypeNull, TypeBoolean, TypeIntegerNumber, TypeFloatNumber, TypeString, TypeBigInt, TypeHole, TypeUninitialized:
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
		case TypeArray:
			// The template object of a tagged template: one per call site
			// and, as in the spec, per realm.
			arr, arrOK := instantiateTemplateObject(k)
			if !arrOK {
				return nil, false
			}
			n.Constants[i] = arr
		default:
			return nil, false
		}
	}
	return &n, true
}

// SharedModuleChunkFallbacks reports how many module chunks this VM had to run
// shared between realms because InstantiateChunk could not copy one of their
// constants. It is zero for ordinary modules; embedders and tests can check it
// to notice a module that is not isolated per realm.
func (vm *VM) SharedModuleChunkFallbacks() int { return vm.sharedModuleChunkFallbacks }

// instantiateTemplateObject copies the frozen template object the compiler
// builds for a tagged template: an array of the cooked strings (undefined for
// an invalid escape) with a frozen array of the raw strings as its `raw`
// property. ok is false for any other array, which this does not know how to
// copy faithfully.
func instantiateTemplateObject(v Value) (Value, bool) {
	src := v.AsArray()
	rawVal, hasRaw := src.GetOwn("raw")
	if !hasRaw || rawVal.Type() != TypeArray || !src.IsFrozen() {
		return Undefined, false
	}
	copyStrings := func(a *ArrayObject, allowUndefined bool) (Value, bool) {
		out := NewArray()
		dst := out.AsArray()
		for i := 0; i < a.Length(); i++ {
			e := a.Get(i)
			if e.Type() != TypeString && !(allowUndefined && e.Type() == TypeUndefined) {
				return Undefined, false
			}
			dst.Append(e)
		}
		dst.SetExtensible(false)
		dst.SetFrozen(true)
		return out, true
	}
	cooked, ok := copyStrings(src, true)
	if !ok {
		return Undefined, false
	}
	raw, ok := copyStrings(rawVal.AsArray(), false)
	if !ok {
		return Undefined, false
	}
	dst := cooked.AsArray()
	dst.DefineOwnProperty("raw", raw, false, false, false)
	return cooked, true
}

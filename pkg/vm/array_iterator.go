package vm

import (
	"strconv"
	"unsafe"
)

// BuiltinIterState is the shared mutable state behind the built-in
// closure-based iterators (array values/keys/entries, array-likes,
// arguments, string). The builtins package hangs it on the
// NativeFunctionObject of the per-iterator next closure; the closure and the
// OpFastIterNext fast path in the dispatch loop both step the same state via
// Step(), so mixing manual it.next() calls with a for-of over the same
// iterator stays coherent.
//
// The for-of fast path is sound because the compiler caches the `next` method
// in a register once per loop (matching the spec's IteratorRecord.[[NextMethod]]
// caching): OpIterFastCheck inspects that cached value once, and a replaced or
// user-defined next simply fails the check and takes the generic call path.
type BuiltinIterState struct {
	Kind      IterKind
	Index     int
	Exhausted bool // Map/Set kinds: sticky done flag (spec [[Exhausted]])

	Arr  *ArrayObject     // IterKindArrayValues/Keys/Entries when the source is a real array
	Args *ArgumentsObject // IterKindArguments
	Like *PlainObject     // array-like source for Keys/Entries/LikeValues (may be nil -> length 0)
	Str  []uint16         // IterKindString: UTF-16 code units of the iterated string
	M    *MapObject       // IterKindMapKeys/Values/Entries
	S    *SetObject       // IterKindSetValues/Entries
}

// IterKind selects what Step yields per iteration.
type IterKind uint8

const (
	IterKindArrayValues IterKind = iota // Arr.Get(i)
	IterKindArrayKeys                   // Number(i) over Arr or Like
	IterKindArrayEntries                // [i, value] pair over Arr or Like
	IterKindArguments                   // Args.Get(i)
	IterKindLikeValues                  // Like[i] via GetOwn
	IterKindString                      // code point (surrogate-pair aware) at UTF-16 index
	IterKindMapKeys                     // M key at insertion-order index (tombstones skipped)
	IterKindMapValues                   // M value at insertion-order index
	IterKindMapEntries                  // [k, v] pair
	IterKindSetValues                   // S value at insertion-order index (keys() aliases this)
	IterKindSetEntries                  // [v, v] pair
	// IterKindStateOnIterator marks the SHARED %MapIteratorPrototype%.next /
	// %SetIteratorPrototype%.next natives: their per-iterator state lives on
	// the iterator PlainObject (InternalIterState), not on the next closure.
	// This sentinel state must never be stepped itself - the fast path
	// resolves the real state from the iterator register instead.
	IterKindStateOnIterator
)

// IsMapKind/IsSetKind report the collection family, used for the prototype
// next brand checks (a Map next called on a Set iterator must throw).
func (k IterKind) IsMapKind() bool {
	return k == IterKindMapKeys || k == IterKindMapValues || k == IterKindMapEntries
}
func (k IterKind) IsSetKind() bool {
	return k == IterKindSetValues || k == IterKindSetEntries
}

// resolveFastIterState returns the steppable iterator state for the for-of
// fast path, or nil when the loop must take the generic call path. State
// normally hangs off the next method's closure (array/string/arguments
// family); for Map/Set iterators the shared prototype next carries the
// IterKindStateOnIterator sentinel and the per-iterator state lives on the
// iterator object itself.
func resolveFastIterState(iterVal, nextVal Value) *BuiltinIterState {
	if nextVal.typ != TypeNativeFunction {
		return nil
	}
	nf := nextVal.AsNativeFunction()
	if nf == nil || nf.IterState == nil {
		return nil
	}
	st := nf.IterState
	if st.Kind != IterKindStateOnIterator {
		if (st.Kind == IterKindArrayValues || st.Kind == IterKindArrayEntries) && st.Arr != nil && st.Arr.HasAccessors() {
			// An own accessor property on the source array (get/set installed
			// via Object.defineProperty) must have its getter called - ordinary
			// [[Get]] semantics, same as plain property access. Step() can't do
			// that: it has no VM to call the getter with and no channel to
			// report a thrown exception, which is exactly why it stays the fast,
			// error-free primitive OpFastIterNext depends on. So bail to the
			// generic iterator.next() call path here instead, which goes
			// through the native `next` closure and StepVM (below) - that path
			// already has both. Checked once per for-of loop (OpIterFastCheck
			// runs before the loop, not per iteration), matching the "checked
			// once, cached" semantics this whole fast-path scheme already uses
			// for the next-method identity check itself.
			return nil
		}
		return st
	}
	if iterVal.typ == TypeObject {
		if po := AsPlainObject(iterVal); po != nil {
			return po.internalIterState
		}
	}
	return nil
}

// isFastDestructureArray reports whether v can be destructured by direct index
// reads instead of the iterator protocol: v must be a plain array whose
// Symbol.iterator is still the canonical Array.prototype iterator. When true,
// default iteration is exactly Arr.Get(0..len-1) (what BuiltinIterState.Step
// yields), so OpArrayRawGetInt reproduces it bit-for-bit. Any instance- or
// prototype-level override of Symbol.iterator fails the identity check and the
// caller falls back to the generic protocol.
//
// Symbol.iterator is resolved the way opGetPropSymbol resolves it — own symbol
// properties, then the per-instance prototype override (`class X extends Array`),
// then the realm's intrinsic Array.prototype and its chain. vm.GetSymbolProperty
// is deliberately not used: its array branch falls back to vm.ArrayPrototype
// unconditionally, so a subclass whose Symbol.iterator lives on X.prototype
// would be missed and this fast path wrongly approved — destructuring would
// silently skip the custom iterator that for-of still honors.
func (vm *VM) isFastDestructureArray(v Value) bool {
	if v.typ != TypeArray || vm.ArrayValuesIterator.typ != TypeNativeFunction {
		return false
	}
	arr := v.AsArray()
	if arr == nil {
		return false
	}

	// An own symbol property shadows everything on the chain.
	if sym := vm.SymbolIterator.AsSymbolObject(); sym != nil {
		if it, ok := arr.GetSymbolProp(sym); ok {
			return vm.isCanonicalArrayValuesIterator(it)
		}
	}

	proto := arr.prototype
	if !proto.IsObject() {
		proto = vm.ArrayPrototype
	}
	symKey := NewSymbolKey(vm.SymbolIterator)
	for cur := proto; cur.IsObject(); {
		po := cur.AsPlainObject()
		if po == nil {
			// A non-plain link (dictionary-mode object) may still carry an
			// override we can't inspect here — take the generic path rather
			// than assume the array is pristine.
			return false
		}
		if it, ok := po.GetOwnByKey(symKey); ok {
			return vm.isCanonicalArrayValuesIterator(it)
		}
		cur = po.prototype
	}
	// No Symbol.iterator anywhere on the chain: not canonical array iteration.
	return false
}

func (vm *VM) isCanonicalArrayValuesIterator(it Value) bool {
	return it.typ == TypeNativeFunction &&
		it.AsNativeFunction() == vm.ArrayValuesIterator.AsNativeFunction()
}

// likeLength reads the array-like's current length the same way the original
// closures did: GetOwn("length") if numeric, else 0.
func (st *BuiltinIterState) likeLength() int {
	if st.Like == nil {
		return 0
	}
	if lenVal, ok := st.Like.GetOwn("length"); ok && lenVal.IsNumber() {
		return int(lenVal.ToFloat())
	}
	return 0
}

// Step advances the iterator by one element and returns (value, done).
// Length/content are re-read from the source every step, so growth,
// truncation, and hole normalization behave exactly like the closures this
// replaces. Called from both the native next closure (which wraps the pair
// in a spec {value, done} object) and the OpFastIterNext opcode (which lands
// value and done directly in registers).
func (st *BuiltinIterState) Step() (Value, bool) {
	switch st.Kind {
	case IterKindArrayValues:
		if st.Index >= st.Arr.Length() {
			return Undefined, true
		}
		v := st.Arr.Get(st.Index)
		st.Index++
		return v, false

	case IterKindArguments:
		if st.Index >= st.Args.Length() {
			return Undefined, true
		}
		v := st.Args.Get(st.Index)
		st.Index++
		return v, false

	case IterKindArrayKeys:
		length := 0
		if st.Arr != nil {
			length = st.Arr.Length()
		} else {
			length = st.likeLength()
		}
		if st.Index >= length {
			return Undefined, true
		}
		v := Number(float64(st.Index))
		st.Index++
		return v, false

	case IterKindArrayEntries:
		length := 0
		if st.Arr != nil {
			length = st.Arr.Length()
		} else {
			length = st.likeLength()
		}
		if st.Index >= length {
			return Undefined, true
		}
		var elem Value = Undefined
		if st.Arr != nil {
			elem = st.Arr.Get(st.Index)
		} else if st.Like != nil {
			if v, ok := st.Like.GetOwn(strconv.Itoa(st.Index)); ok {
				elem = v
			}
		}
		pair := NewArray()
		pairArr := pair.AsArray()
		pairArr.Append(Number(float64(st.Index)))
		pairArr.Append(elem)
		st.Index++
		return pair, false

	case IterKindLikeValues:
		if st.Index >= st.likeLength() {
			return Undefined, true
		}
		var v Value = Undefined
		if st.Like != nil {
			if pv, ok := st.Like.GetOwn(strconv.Itoa(st.Index)); ok {
				v = pv
			}
		}
		st.Index++
		return v, false

	case IterKindString:
		if st.Index >= len(st.Str) {
			return Undefined, true
		}
		// ECMAScript string iteration yields code points: combine a valid
		// surrogate pair into one result, pass lone surrogates through
		// (UTF16ToString preserves them via WTF-8).
		c := st.Str[st.Index]
		n := 1
		if c >= 0xD800 && c <= 0xDBFF && st.Index+1 < len(st.Str) {
			if low := st.Str[st.Index+1]; low >= 0xDC00 && low <= 0xDFFF {
				n = 2
			}
		}
		v := NewString(UTF16ToString(st.Str[st.Index : st.Index+n]))
		st.Index += n
		return v, false

	case IterKindMapKeys, IterKindMapValues, IterKindMapEntries:
		// Live iteration over insertion order: entries deleted during
		// iteration are skipped (tombstones), entries added during
		// iteration are visited. Matches the previous slot-property-based
		// %MapIteratorPrototype%.next exactly, including advancing Index
		// past tombstones and the sticky Exhausted flag.
		if st.Exhausted {
			return Undefined, true
		}
		for st.Index < st.M.OrderLen() {
			key, value, exists := st.M.GetEntryAt(st.Index)
			st.Index++
			if exists {
				switch st.Kind {
				case IterKindMapKeys:
					return key, false
				case IterKindMapValues:
					return value, false
				default: // entries
					entry := NewArray()
					entryArr := entry.AsArray()
					entryArr.Append(key)
					entryArr.Append(value)
					return entry, false
				}
			}
		}
		st.Exhausted = true
		return Undefined, true

	case IterKindSetValues, IterKindSetEntries:
		if st.Exhausted {
			return Undefined, true
		}
		for st.Index < st.S.OrderLen() {
			value, exists := st.S.GetValueAt(st.Index)
			st.Index++
			if exists {
				if st.Kind == IterKindSetEntries {
					entry := NewArray()
					entryArr := entry.AsArray()
					entryArr.Append(value)
					entryArr.Append(value)
					return entry, false
				}
				return value, false
			}
		}
		st.Exhausted = true
		return Undefined, true
	}
	return Undefined, true
}

// getOwnIndexed reads the array's index i via ordinary [[Get]]: an own
// accessor property at that index (get/set installed via
// Object.defineProperty, at any index - DefineAccessorProperty never touches
// `elements`, so an accessor index's raw slot is stale or a leftover Hole,
// not the value [[Get]] must produce) has its getter called with the array
// itself as `this` (a setter-only accessor reads as undefined, matching
// ordinary [[Get]] on an accessor with no getter, without calling anything);
// otherwise falls back to the raw element via Get, which is already
// bounds/hole safe. Mirrors the accessor-first-then-raw-element precedence
// arrayLikeGet (pkg/builtins/array_generic.go) uses for the generic
// Array.prototype methods, applied here for the shared array values/entries
// iterator state that backs for-of, spread's generic iterator-protocol
// fallback, and iterator-protocol destructuring - including rest
// (`const [x, ...rest] = arr`), which always takes the full generic path
// since a rest element disables the destructuring fast path
// (arrayDeclPatternFastEligible).
func (a *ArrayObject) getOwnIndexed(vmInstance *VM, i int) (Value, error) {
	key := strconv.Itoa(i)
	if getter, _, _, _, ok := a.GetOwnAccessor(key); ok {
		if getter.Type() == TypeUndefined {
			return Undefined, nil
		}
		return vmInstance.Call(getter, Value{typ: TypeArray, obj: unsafe.Pointer(a)}, nil)
	}
	return a.Get(i), nil
}

// StepVM behaves exactly like Step, except for IterKindArrayValues and
// IterKindArrayEntries when the source array has at least one own accessor
// property: those two kinds read an element via getOwnIndexed instead of
// Step()'s raw Arr.Get, so an own accessor's getter runs. Step() itself
// can't do this - it has no VM to call the getter with and no way to report
// a thrown exception - which is exactly why callers that can only tolerate
// the fast, error-free Step() (the OpFastIterNext dispatch-loop opcode) bail
// to the generic iterator-protocol call path up front instead, via
// resolveFastIterState's own HasAccessors check. StepVM is for the one
// caller that already goes through a real call and has an error channel to
// use: the native `next` closure (makeBuiltinIterNext).
//
// A getter is arbitrary script: it can shrink the array's own backing
// storage mid-loop (`a.length = 0`, `a.pop()`, ...). No extra guard is
// needed for that here, the same way Step()'s own IterKindArrayValues case
// needs none - Length() and getOwnIndexed's Get fallback are both re-read
// live on every step, so a shrunk array simply reports done sooner instead
// of a Go slice-bounds panic.
func (st *BuiltinIterState) StepVM(vmInstance *VM) (Value, bool, error) {
	switch st.Kind {
	case IterKindArrayValues:
		if st.Arr == nil || !st.Arr.HasAccessors() {
			v, done := st.Step()
			return v, done, nil
		}
		if st.Index >= st.Arr.Length() {
			return Undefined, true, nil
		}
		idx := st.Index
		st.Index++
		v, err := st.Arr.getOwnIndexed(vmInstance, idx)
		if err != nil {
			return Undefined, false, err
		}
		return v, false, nil

	case IterKindArrayEntries:
		if st.Arr == nil || !st.Arr.HasAccessors() {
			v, done := st.Step()
			return v, done, nil
		}
		if st.Index >= st.Arr.Length() {
			return Undefined, true, nil
		}
		idx := st.Index
		st.Index++
		elem, err := st.Arr.getOwnIndexed(vmInstance, idx)
		if err != nil {
			return Undefined, false, err
		}
		pair := NewArray()
		pairArr := pair.AsArray()
		pairArr.Append(Number(float64(idx)))
		pairArr.Append(elem)
		return pair, false, nil

	default:
		v, done := st.Step()
		return v, done, nil
	}
}

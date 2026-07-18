package vm

import "strconv"

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
		return st
	}
	if iterVal.typ == TypeObject {
		if po := AsPlainObject(iterVal); po != nil {
			return po.internalIterState
		}
	}
	return nil
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

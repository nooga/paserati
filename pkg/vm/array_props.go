package vm

// HasOwnIndexProperty reports whether a numeric index is a genuine own
// property of the array - the same three-way check ArrayObject.Get's
// callers (hasOwnProperty, `in`, Object.keys/values/entries/
// getOwnPropertyNames, Reflect.ownKeys, Object.assign,
// getOwnPropertyDescriptor, and the Proxy invariant checks that mirror it)
// must all agree on, or a hole (from `delete arr[i]`, a literal elision, or
// `new Array(n)`) gets reported as a present property whose value happens
// to be undefined (paserati#300):
//
//  1. An accessor explicitly installed at this index via
//     Object.defineProperty takes priority - it's tracked separately from
//     `elements` (see ArrayDefineOwnProperty's doc comment).
//  2. Otherwise, HasIndex covers the dense `elements` range, correctly
//     reporting a hole as absent.
//  3. Beyond that range (see maxDenseArrayDefineIndex), a huge sparse index
//     defined via Object.defineProperty (paserati#176/#178) is tracked in
//     the properties map instead of `elements`, so it must still count as
//     present even though HasIndex can't see it.
//
// propName must be index formatted as a decimal string (the same key
// GetOwnAccessor/GetOwn use); callers that already have both the parsed int
// and its string form pass both to avoid re-formatting.
func (a *ArrayObject) HasOwnIndexProperty(propName string, index int) bool {
	if index < 0 {
		return false
	}
	if a.HasAccessors() {
		if _, _, _, _, ok := a.GetOwnAccessor(propName); ok {
			return true
		}
	}
	if a.HasIndex(index) {
		return true
	}
	_, ok := a.GetOwn(propName)
	return ok
}

// maxDenseArrayDefineIndex bounds how far ArrayDefineOwnProperty will grow
// the elements slice via ArrayObject.Set for a single index write. A valid
// array index can be as large as 2^32-2 (ArraySetLength's bound - see
// tryParseArrayIndex), and Set(idx, ...) is O(idx): it fills every slot up
// to idx with Hole before writing. Without this guard, defining a property
// at a huge index (a real Test262 boundary-condition pattern, e.g. testing
// behavior at 4294967294) would attempt a multi-billion-entry allocation -
// mirrors OpSetIndex's own identical guard in vm.go for `arr[hugeIdx] = v`.
const maxDenseArrayDefineIndex = 16777216 // 2^24

// ArrayPrototypeSetterFor checks whether idx has an inherited accessor
// somewhere on Array.prototype's chain - an own-index write on an array
// with no own property there must still consult the prototype first
// (ECMA-262 9.1.9.2 OrdinarySetWithOwnDescriptor step 2's "ownDesc is
// undefined -> walk to the parent" rule) rather than unconditionally
// creating a new own data property. Returns (handled, err): handled is
// true if a setter was invoked (delivering val that way) or a getter-only
// accessor silently swallowed the write (matching sloppy-mode semantics,
// since neither OpSetIndex nor arrayLikeSet's callers track strict mode);
// false means the caller should fall through to a plain own-property
// write.
//
// Guarded behind arrayIndexAccessorSeen (object.go) the same way this
// logic always was, back when it only existed inline in OpSetIndex:
// integer-indexed accessors on Array.prototype essentially never exist, so
// the whole walk (a string-key format + chain scan on every element write)
// is skipped until one is actually defined. Factored out here so
// OpSetIndex (the `arr[i] = v` bytecode fast path) and arrayLikeSet
// (package builtins - Array.prototype.push/unshift/... writing new
// indices) share one implementation instead of two that could drift.
func (vm *VM) ArrayPrototypeSetterFor(receiver Value, idx int, val Value) (bool, error) {
	if !arrayIndexAccessorSeen.Load() || !vm.ArrayPrototype.IsObject() {
		return false, nil
	}
	setterKey := "s:" + intToString(idx) // PropertyKey hash format for string keys
	for cur := vm.ArrayPrototype.AsPlainObject(); cur != nil; {
		if cur.setters != nil {
			if s, ok := cur.setters[setterKey]; ok {
				_, err := vm.Call(s, receiver, []Value{val})
				return true, err
			}
		}
		if cur.getters != nil {
			if _, ok := cur.getters[setterKey]; ok {
				return true, nil
			}
		}
		protoVal := cur.GetPrototype()
		if protoVal.Type() != TypeObject {
			break
		}
		cur = protoVal.AsPlainObject()
	}
	return false, nil
}

// ArrayDefineOwnProperty implements the Array exotic object's
// [[DefineOwnProperty]] (ECMA-262 10.4.2.1, deferring to
// OrdinaryDefineOwnProperty 10.1.6.3 for everything but the "length" key,
// which callers must handle separately - see below) for one property key,
// covering both a numeric index (an element of the array) and an arbitrary
// named property.
//
// The elements slice itself holds only values (or Hole for a sparse gap),
// not a writable/enumerable/configurable triple per slot - so a plain data
// element's descriptor defaults to the ES default (value, true, true, true)
// when nothing has ever said otherwise. An explicit Object.defineProperty
// call that requests a non-default combination for an in-range index is
// tracked in propertyDesc instead (keyed by the index's string form, same
// map a named property or an accessor index already uses - see
// ArrayObject.DefineAccessorProperty) - GetOwnPropertyDescriptor,
// DeleteIndex, and this function's own re-entry all check it first before
// falling back to the default (paserati#178; a plain `arr[i] = v` never
// goes through this function and so never creates an entry, keeping the
// common case free of any per-element bookkeeping). This codebase still
// doesn't enforce non-writable/non-configurable at the *whole-array*
// level any more precisely than a single `frozen` flag (Object.freeze),
// independent of this per-index tracking.
//
// Mirrors ArgumentsDefineOwnProperty's structure (arguments_props.go) for a
// different exotic object with the same shape of problem: numeric indices
// need customized [[DefineOwnProperty]] behavior beyond a bare property
// bag. This does not implement Array's "length" key at all (ArraySetLength,
// 10.4.2.4, requires re-deriving/possibly truncating elements top-down,
// rejecting invalid uint32 lengths, and stopping partway through a
// non-configurable element) - callers must special-case "length" and skip
// this function for it, same as ArgumentsDefineOwnProperty's callers
// special-case "length"/"callee".
func (vm *VM) ArrayDefineOwnProperty(
	a *ArrayObject, key string,
	hasValue bool, value Value,
	writablePtr, enumerablePtr, configurablePtr *bool,
	hasGetter bool, getter Value,
	hasSetter bool, setter Value,
) error {
	becomingAccessor := hasGetter || hasSetter
	idx, isIndex := tryParseArrayIndex(key)

	// Resolve the current descriptor, in priority order: a tracked accessor
	// (numeric or named), a plain in-bounds element, a tracked named data
	// property. Whichever matches (or none) sets exists/isAccessor/curX.
	var exists, isAccessor, curWritable, curEnumerable, curConfigurable bool
	if _, _, e, c, ok := a.GetOwnAccessor(key); ok {
		exists, isAccessor, curEnumerable, curConfigurable = true, true, e, c
	} else if isIndex && idx < len(a.elements) && a.elements[idx].typ != TypeHole {
		// The ES default for a plain element (paserati#178: a prior
		// ArrayDefineOwnProperty call on this same index may have tracked a
		// non-default combination in propertyDesc instead - see the write
		// side below - and DeleteIndex already checks propertyDesc first
		// for exactly this reason).
		exists, curWritable, curEnumerable, curConfigurable = true, true, true, true
		if a.propertyDesc != nil {
			if desc, ok := a.propertyDesc[key]; ok {
				curWritable, curEnumerable, curConfigurable = desc.Writable, desc.Enumerable, desc.Configurable
			}
		}
	} else if _, desc, ok := a.GetOwnPropertyDescriptor(key); ok {
		exists, curWritable, curEnumerable, curConfigurable = true, desc.Writable, desc.Enumerable, desc.Configurable
	}

	// OrdinaryDefineOwnProperty 10.1.6.3 steps 3-4: reject on a
	// non-configurable existing property whose descriptor the caller is
	// trying to widen or whose kind/writability it's trying to narrow.
	if exists && !curConfigurable {
		if configurablePtr != nil && *configurablePtr {
			return vm.NewTypeError("Cannot redefine property: " + key)
		}
		if enumerablePtr != nil && *enumerablePtr != curEnumerable {
			return vm.NewTypeError("Cannot redefine property: " + key)
		}
		if isAccessor && (hasValue || writablePtr != nil) {
			return vm.NewTypeError("Cannot redefine property: " + key)
		}
		if !isAccessor && becomingAccessor {
			return vm.NewTypeError("Cannot redefine property: " + key)
		}
		if !isAccessor && !becomingAccessor && !curWritable && writablePtr != nil && *writablePtr {
			return vm.NewTypeError("Cannot redefine property: " + key)
		}
	} else if !exists && !a.IsExtensible() {
		return vm.NewTypeError("Cannot define property " + key + ", object is not extensible")
	}

	enumerable := curEnumerable
	if !exists {
		enumerable = false
	}
	if enumerablePtr != nil {
		enumerable = *enumerablePtr
	}
	configurable := curConfigurable
	if !exists {
		configurable = false
	}
	if configurablePtr != nil {
		configurable = *configurablePtr
	}

	if becomingAccessor {
		a.DefineAccessorProperty(key, getter, hasGetter, setter, hasSetter, &enumerable, &configurable)
		if isIndex {
			// Every read path that knows about per-index accessors
			// (arrayLikeGet/Set in package builtins, opGetProp,
			// OpGetIndex/OpSetIndex) checks GetOwnAccessor before ever
			// consulting the elements slice, so nothing needs to be written
			// there. Just grow the tracked length the way defining an
			// array-index property past the end would (ArraySetLength) -
			// a bare int bump, safe for any idx up to the 2^32-2 bound
			// (unlike touching `elements`, see maxDenseArrayDefineIndex).
			if idx+1 > a.length {
				a.length = idx + 1
			}
		}
		return nil
	}

	writable := curWritable
	if !exists {
		writable = false
	}
	if writablePtr != nil {
		writable = *writablePtr
	}
	newValue := value
	if !hasValue {
		if isIndex {
			newValue = a.Get(idx)
		} else if v, ok := a.GetOwn(key); ok {
			newValue = v
		} else {
			newValue = Undefined
		}
	}

	if isIndex {
		// Converting an accessor index back to a data property: drop the
		// tracked accessor state so GetOwnAccessor stops shadowing it.
		if a.getters != nil {
			delete(a.getters, key)
		}
		if a.setters != nil {
			delete(a.setters, key)
		}
		if a.propertyDesc != nil {
			delete(a.propertyDesc, key)
		}
		if idx <= maxDenseArrayDefineIndex {
			a.Set(idx, newValue)
			// A plain element's descriptor otherwise reports the ES
			// default (value, true, true, true) - see this file's doc
			// comment. Track a non-default writable/enumerable/
			// configurable combination explicitly in propertyDesc instead
			// (paserati#178) - GetOwnPropertyDescriptor, DeleteIndex, and
			// this function's own "resolve current descriptor" step above
			// all already check propertyDesc first for exactly this
			// reason. An all-default combination needs no entry, matching
			// the delete just above (which also clears whatever an
			// earlier defineProperty call on this index may have left).
			if !(writable && enumerable && configurable) {
				if a.propertyDesc == nil {
					a.propertyDesc = make(map[string]PropertyDesc)
				}
				a.propertyDesc[key] = PropertyDesc{Writable: writable, Enumerable: enumerable, Configurable: configurable}
			}
		} else {
			// Beyond the dense-allocation bound: track it as a named
			// property instead of materializing the elements slice up to
			// idx (see maxDenseArrayDefineIndex). DefineOwnProperty stores
			// the actual attributes here (unlike the old hardcoded
			// true/true/true - paserati#178), since it always tracks a
			// full descriptor for a named property regardless of index
			// size.
			a.DefineOwnProperty(key, newValue, writable, enumerable, configurable)
			if idx+1 > a.length {
				a.length = idx + 1
			}
		}
		return nil
	}

	a.DefineOwnProperty(key, newValue, writable, enumerable, configurable)
	return nil
}

// ArrayDefineOwnSymbolProperty implements the Array exotic object's
// [[DefineOwnProperty]] (ECMA-262 10.4.2.1 -> OrdinaryDefineOwnProperty
// 10.1.6.3) for a symbol-keyed property. This is the symbol-keyed
// counterpart of ArrayDefineOwnProperty above, backing
// Object.defineProperty(arr, sym, {...}) - previously a silent no-op for
// arrays: a symbol key never matched any of ArrayDefineOwnProperty's
// callers' branches (all gated on a non-symbol propName), and there was no
// storage to hold a non-default attribute combination or an accessor even if
// one had been dispatched here. A symbol key never aliases a numeric index
// or "length" the way a named key can, so none of ArrayDefineOwnProperty's
// index/length-specific machinery (dense-element growth, sparse-index
// tracking, tryParseArrayIndex) applies - this mirrors only that function's
// plain named-property tail (the non-index path).
func (vm *VM) ArrayDefineOwnSymbolProperty(
	a *ArrayObject, sym *SymbolObject,
	hasValue bool, value Value,
	writablePtr, enumerablePtr, configurablePtr *bool,
	hasGetter bool, getter Value,
	hasSetter bool, setter Value,
) error {
	becomingAccessor := hasGetter || hasSetter

	var exists, isAccessor, curWritable, curEnumerable, curConfigurable bool
	if _, _, e, c, ok := a.GetOwnSymbolAccessor(sym); ok {
		exists, isAccessor, curEnumerable, curConfigurable = true, true, e, c
	} else if _, desc, ok := a.GetSymbolPropertyDescriptor(sym); ok {
		exists, curWritable, curEnumerable, curConfigurable = true, desc.Writable, desc.Enumerable, desc.Configurable
	}

	// OrdinaryDefineOwnProperty 10.1.6.3 steps 3-4: reject on a
	// non-configurable existing property whose descriptor the caller is
	// trying to widen or whose kind/writability it's trying to narrow.
	if exists && !curConfigurable {
		if configurablePtr != nil && *configurablePtr {
			return vm.NewTypeError("Cannot redefine property: Symbol()")
		}
		if enumerablePtr != nil && *enumerablePtr != curEnumerable {
			return vm.NewTypeError("Cannot redefine property: Symbol()")
		}
		if isAccessor && (hasValue || writablePtr != nil) {
			return vm.NewTypeError("Cannot redefine property: Symbol()")
		}
		if !isAccessor && becomingAccessor {
			return vm.NewTypeError("Cannot redefine property: Symbol()")
		}
		if !isAccessor && !becomingAccessor && !curWritable && writablePtr != nil && *writablePtr {
			return vm.NewTypeError("Cannot redefine property: Symbol()")
		}
		// NOT checked here, matching ArrayDefineOwnProperty's identical
		// pre-existing gap for named keys (10.1.6.3 step 4e-ii): redefining
		// a non-configurable, non-writable data property with a *different*
		// [[Value]] should also throw, but doesn't - it's silently accepted
		// as a no-op-that-isn't since the write below still updates the
		// value if hasValue is true. Left this way for parity with the
		// named-key path rather than fixing only the symbol-key side.
	} else if !exists && !a.IsExtensible() {
		return vm.NewTypeError("Cannot define property Symbol(), object is not extensible")
	}

	enumerable := curEnumerable
	if !exists {
		enumerable = false
	}
	if enumerablePtr != nil {
		enumerable = *enumerablePtr
	}
	configurable := curConfigurable
	if !exists {
		configurable = false
	}
	if configurablePtr != nil {
		configurable = *configurablePtr
	}

	if becomingAccessor {
		a.DefineSymbolAccessorProperty(sym, getter, hasGetter, setter, hasSetter, &enumerable, &configurable)
		return nil
	}

	writable := curWritable
	if !exists {
		writable = false
	}
	if writablePtr != nil {
		writable = *writablePtr
	}
	newValue := value
	if !hasValue {
		if v, ok := a.GetSymbolProp(sym); ok {
			newValue = v
		} else {
			newValue = Undefined
		}
	}

	a.DefineSymbolProperty(sym, newValue, writable, enumerable, configurable)
	return nil
}

// DeleteIndex implements [[Delete]] (ECMA-262 10.4.2.1 -> OrdinaryDelete
// 10.1.7) for a numeric array index, clearing the slot (and any tracked
// accessor/descriptor state for it) and reporting success. A plain element
// has no per-index configurable bit of its own (see this file's doc
// comment on why) - its configurability instead comes from the array-wide
// `frozen` flag (Object.freeze - SetFrozen), treated as making every
// element non-configurable, same as this codebase's other frozen-array
// checks (e.g. IsFrozen). An index that was turned into an accessor (or
// otherwise given an explicit descriptor - see ArrayDefineOwnProperty) has
// its own tracked configurable bit in propertyDesc and is checked instead.
//
// Shared by OpDeleteIndex (`delete arr[i]` in JS) and arrayLikeDelete
// (array_generic.go, package builtins - copyWithin/splice/etc.'s
// DeletePropertyOrThrow) so both report identical success/failure.
func (a *ArrayObject) DeleteIndex(idx int) bool {
	key := intToString(idx)
	configurable := !a.frozen
	if a.propertyDesc != nil {
		if desc, ok := a.propertyDesc[key]; ok {
			// Object.freeze only ever flips the array-wide `frozen` flag,
			// never touching propertyDesc (see ArrayDefineOwnProperty's doc
			// comment) - so a tracked configurable:true from before the
			// freeze must still be ANDed with !a.frozen here (paserati#178):
			// freeze can only take the capability away, never hand back one
			// an explicit defineProperty granted. Before #178 tracked a
			// plain data index's own attributes at all, this branch could
			// only ever fire for an accessor index, where the same
			// intersection already applies for the same reason.
			configurable = desc.Configurable && !a.frozen
		}
	}
	if !configurable {
		return false
	}
	if a.getters != nil {
		delete(a.getters, key)
	}
	if a.setters != nil {
		delete(a.setters, key)
	}
	if a.propertyDesc != nil {
		delete(a.propertyDesc, key)
	}
	if a.properties != nil {
		delete(a.properties, key)
	}
	if idx < len(a.elements) {
		a.elements[idx] = Hole
	}
	return true
}

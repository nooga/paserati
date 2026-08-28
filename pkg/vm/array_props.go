package vm

// maxDenseArrayDefineIndex bounds how far ArrayDefineOwnProperty will grow
// the elements slice via ArrayObject.Set for a single index write. A valid
// array index can be as large as 2^32-2 (ArraySetLength's bound - see
// tryParseArrayIndex), and Set(idx, ...) is O(idx): it fills every slot up
// to idx with Hole before writing. Without this guard, defining a property
// at a huge index (a real Test262 boundary-condition pattern, e.g. testing
// behavior at 4294967294) would attempt a multi-billion-entry allocation -
// mirrors OpSetIndex's own identical guard in vm.go for `arr[hugeIdx] = v`.
const maxDenseArrayDefineIndex = 16777216 // 2^24

// ArrayDefineOwnProperty implements the Array exotic object's
// [[DefineOwnProperty]] (ECMA-262 10.4.2.1, deferring to
// OrdinaryDefineOwnProperty 10.1.6.3 for everything but the "length" key,
// which callers must handle separately - see below) for one property key,
// covering both a numeric index (an element of the array) and an arbitrary
// named property.
//
// Array elements have no per-index attribute storage: the elements slice
// holds only values (or Hole for a sparse gap), not a writable/enumerable/
// configurable triple per slot. So a plain data element's "current"
// descriptor is always reported as the ES default (value, true, true,
// true) - this codebase's existing posture of not enforcing per-element
// non-writable/non-configurable (see e.g. Object.freeze's array handling,
// which sets a single array-wide `frozen` flag rather than per-index
// descriptors). An index that has been explicitly turned into an accessor
// *is* tracked precisely, through the same getters/setters/propertyDesc
// maps a named property uses (see ArrayObject.DefineAccessorProperty).
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
		exists, curWritable, curEnumerable, curConfigurable = true, true, true, true
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
		} else {
			// Beyond the dense-allocation bound: track it as a named
			// property instead of materializing the elements slice up to
			// idx (see maxDenseArrayDefineIndex). Known gap, shared with
			// OpSetIndex's identical fallback: a direct `arr[idx]`/
			// arrayLikeGet read for an index this large that's still below
			// the (now-grown) `length` hits the elements-array fast path
			// first and misses this map, rather than falling through to
			// the named-property lookup that would find it. Rare enough in
			// practice (an index this close to the 2^32-2 boundary) that
			// this codebase accepts it elsewhere rather than adding a
			// sparse-index data structure.
			a.DefineOwnProperty(key, newValue, true, true, true)
			if idx+1 > a.length {
				a.length = idx + 1
			}
		}
		// writable/enumerable/configurable for a plain element have nowhere
		// to live (see doc comment) - dropped here, matching this
		// codebase's existing non-enforcement of per-element attributes.
		return nil
	}

	a.DefineOwnProperty(key, newValue, writable, enumerable, configurable)
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
			configurable = desc.Configurable
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

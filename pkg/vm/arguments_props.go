package vm

import "strconv"

// This file centralizes the arguments exotic object's [[GetOwnProperty]],
// [[DefineOwnProperty]], [[Get]], [[Set]], and [[Delete]] behavior (ES
// 10.4.4), so every call site (bytecode fast paths, the native GetProperty/
// SetProperty API, and Object.defineProperty/getOwnPropertyDescriptor) shares
// one source of truth instead of independently re-deriving per-index
// defaults. Before this file existed, six different call sites each
// hardcoded {writable:true, enumerable:true, configurable:true} for numeric
// indices and none of them consulted (or even could record) a
// defineProperty override - see language/arguments-object/mapped/*.

// ArgOwnResult is the resolved own-property state for one key on an
// arguments object, before any user code (a getter) has been invoked.
type ArgOwnResult struct {
	Exists       bool
	IsAccessor   bool
	Value        Value // meaningful when !IsAccessor
	Getter       Value
	Setter       Value
	HasGetter    bool
	HasSetter    bool
	Writable     bool // meaningful when !IsAccessor
	Enumerable   bool
	Configurable bool
}

// isLiveMappedIndex reports whether index is still write-through-linked to
// its parameter register - i.e. it's a mapped index whose binding hasn't
// been severed by a prior defineProperty (writable:false or an accessor
// conversion; ES 10.4.4.7 step 6).
func (a *ArgumentsObject) isLiveMappedIndex(index int, override *ArgDescriptor) bool {
	if index < 0 || index >= a.numMapped || a.mappedRegs == nil {
		return false
	}
	if override != nil && override.Unmapped {
		return false
	}
	return true
}

// ArgumentsOwnProperty resolves the current own-property descriptor for key,
// without invoking any user code. Numeric-index keys default to
// {writable:true, enumerable:true, configurable:true} per
// CreateMappedArgumentsObject; "length" and "callee" are handled by their
// own dedicated call sites and are intentionally not covered here.
func (a *ArgumentsObject) ArgumentsOwnProperty(key string) ArgOwnResult {
	if a.deletedProps != nil && a.deletedProps[key] {
		return ArgOwnResult{Exists: false}
	}

	var override *ArgDescriptor
	if a.argDescs != nil {
		override = a.argDescs[key]
	}

	index, isIndex := tryParseArgIndex(key)

	if override != nil {
		if override.IsAccessor {
			return ArgOwnResult{
				Exists: true, IsAccessor: true,
				Getter: override.Getter, Setter: override.Setter,
				HasGetter: override.HasGetter, HasSetter: override.HasSetter,
				Enumerable: override.Enumerable, Configurable: override.Configurable,
			}
		}
		value := override.Value
		if isIndex && a.isLiveMappedIndex(index, override) {
			// Still mapped: the live register is authoritative, not the
			// (possibly stale) stored value - a plain `arguments[i] = x` or
			// `a = x` assignment writes the register directly and doesn't
			// go through argDescs.
			value = a.mappedRegs[index]
		}
		return ArgOwnResult{
			Exists: true, Value: value,
			Writable: override.Writable, Enumerable: override.Enumerable, Configurable: override.Configurable,
		}
	}

	// No override recorded: synthesize the default descriptor for whichever
	// kind of key this is.
	if isIndex {
		if index < a.numMapped && a.mappedRegs != nil {
			return ArgOwnResult{Exists: true, Value: a.mappedRegs[index], Writable: true, Enumerable: true, Configurable: true}
		}
		if index < len(a.args) {
			return ArgOwnResult{Exists: true, Value: a.args[index], Writable: true, Enumerable: true, Configurable: true}
		}
		if a.namedProps != nil {
			if v, ok := a.namedProps[key]; ok {
				return ArgOwnResult{Exists: true, Value: v, Writable: true, Enumerable: true, Configurable: true}
			}
		}
		return ArgOwnResult{Exists: false}
	}

	// Non-index overflow property added via SetNamedProp without ever going
	// through defineProperty (e.g. `arguments.foo = 1`).
	if a.namedProps != nil {
		if v, ok := a.namedProps[key]; ok {
			return ArgOwnResult{Exists: true, Value: v, Writable: true, Enumerable: true, Configurable: true}
		}
	}
	return ArgOwnResult{Exists: false}
}

// tryParseArgIndex parses key as a non-negative array index. Mirrors
// tryParseArrayIndex's contract but lives here to keep this file
// self-contained.
func tryParseArgIndex(key string) (int, bool) {
	idx, err := strconv.Atoi(key)
	if err != nil || idx < 0 {
		return 0, false
	}
	return idx, true
}

// ParseArgumentsIndex is the exported form of tryParseArgIndex, for callers
// outside this package (e.g. pkg/builtins) that need to tell whether a
// property key is a numeric-index key on an arguments object before
// invoking ArgumentsDefineOwnProperty.
func ParseArgumentsIndex(key string) (int, bool) { return tryParseArgIndex(key) }

// argumentsGet resolves key's value for [[Get]], calling the getter if key
// was converted to an accessor property.
func (vm *VM) argumentsGet(a *ArgumentsObject, key string) (Value, error) {
	own := a.ArgumentsOwnProperty(key)
	if !own.Exists {
		return Undefined, nil
	}
	if own.IsAccessor {
		if !own.HasGetter || own.Getter.Type() == TypeUndefined {
			return Undefined, nil
		}
		return vm.Call(own.Getter, NewValueFromArguments(a), nil)
	}
	return own.Value, nil
}

// argumentsSet implements [[Set]] for a numeric-index or overflow-named key,
// including write-through to the live parameter register while still
// mapped. strict controls whether a rejected write (non-writable, or a
// data-only accessor with no setter) throws or silently no-ops.
func (vm *VM) argumentsSet(a *ArgumentsObject, key string, value Value, strict bool) error {
	own := a.ArgumentsOwnProperty(key)

	if own.Exists && own.IsAccessor {
		if own.HasSetter && own.Setter.Type() != TypeUndefined {
			_, err := vm.Call(own.Setter, NewValueFromArguments(a), []Value{value})
			return err
		}
		if strict {
			return vm.NewTypeError("Cannot set property " + key + " of arguments which has only a getter")
		}
		return nil
	}

	if own.Exists && !own.Writable {
		if strict {
			return vm.NewTypeError("Cannot assign to read only property '" + key + "' of object '[object Arguments]'")
		}
		return nil
	}

	index, isIndex := tryParseArgIndex(key)
	var override *ArgDescriptor
	if a.argDescs != nil {
		override = a.argDescs[key]
	}

	if isIndex && a.isLiveMappedIndex(index, override) {
		a.mappedRegs[index] = value
		// Keep a same override's stored value in sync too, purely so a
		// later read that (incorrectly) bypassed isLiveMappedIndex still
		// sees something sane; ArgumentsOwnProperty itself always prefers
		// the live register while mapped.
		if override != nil {
			override.Value = value
		}
		return nil
	}

	if override != nil {
		override.Value = value
		return nil
	}

	if isIndex {
		a.SetIndexed(index, value)
		return nil
	}
	a.SetNamedProp(key, value)
	return nil
}

// argumentsDefineOwnProperty implements ES 10.4.4.7 ArgumentsExoticObjects
// [[DefineOwnProperty]] for a numeric-index key: validates the change
// against the current descriptor exactly like OrdinaryDefineOwnProperty,
// then applies ES step 6's mapped-argument side effects (write-through the
// new value while still mapped; sever the mapping if the property becomes
// non-writable or is converted to an accessor).
func (vm *VM) ArgumentsDefineOwnProperty(
	a *ArgumentsObject, key string,
	hasValue bool, value Value,
	writablePtr, enumerablePtr, configurablePtr *bool,
	hasGetter bool, getter Value,
	hasSetter bool, setter Value,
) error {
	current := a.ArgumentsOwnProperty(key)
	becomingAccessor := hasGetter || hasSetter

	if current.Exists && !current.Configurable {
		if configurablePtr != nil && *configurablePtr {
			return vm.NewTypeError("Cannot redefine property: " + key)
		}
		if enumerablePtr != nil && *enumerablePtr != current.Enumerable {
			return vm.NewTypeError("Cannot redefine property: " + key)
		}
		if current.IsAccessor && (hasValue || writablePtr != nil) {
			return vm.NewTypeError("Cannot redefine property: " + key)
		}
		if !current.IsAccessor && becomingAccessor {
			return vm.NewTypeError("Cannot redefine property: " + key)
		}
		if !current.IsAccessor && !becomingAccessor {
			if !current.Writable && writablePtr != nil && *writablePtr {
				return vm.NewTypeError("Cannot redefine property: " + key)
			}
			if !current.Writable && hasValue && !value.Is(current.Value) {
				return vm.NewTypeError("Cannot redefine property: " + key)
			}
		}
	} else if !current.Exists {
		if a.argDescs == nil {
			a.argDescs = make(map[string]*ArgDescriptor)
		}
	}

	// Merge onto the resolved current state - missing fields are preserved,
	// matching OrdinaryDefineOwnProperty's "fields not present in Desc keep
	// the current value" rule (defaulting to the ES defaults when the
	// property is being created for the first time).
	writable := current.Writable
	if !current.Exists {
		writable = true
	}
	if writablePtr != nil {
		writable = *writablePtr
	}
	enumerable := current.Enumerable
	if !current.Exists {
		enumerable = true
	}
	if enumerablePtr != nil {
		enumerable = *enumerablePtr
	}
	configurable := current.Configurable
	if !current.Exists {
		configurable = true
	}
	if configurablePtr != nil {
		configurable = *configurablePtr
	}

	newDesc := &ArgDescriptor{
		Enumerable: enumerable, Configurable: configurable,
	}

	index, isIndex := tryParseArgIndex(key)
	existingOverride := a.argDescIfPresent(key)
	wasLiveMapped := isIndex && a.isLiveMappedIndex(index, existingOverride)

	// Once a mapped index's binding is severed it stays severed - this
	// defineProperty call may not even touch writable/accessor-ness (e.g. a
	// second call only setting configurable:false), in which case the
	// "wasLiveMapped" branches below never run at all and mustn't leave
	// Unmapped at its bool zero value (false), silently un-severing a
	// mapping a prior call already removed.
	if isIndex {
		if existingOverride != nil {
			newDesc.Unmapped = existingOverride.Unmapped
		} else {
			newDesc.Unmapped = !(index < a.numMapped && a.mappedRegs != nil)
		}
	}

	if becomingAccessor {
		newDesc.IsAccessor = true
		if hasGetter {
			newDesc.Getter = getter
			newDesc.HasGetter = true
		} else if current.IsAccessor {
			newDesc.Getter = current.Getter
			newDesc.HasGetter = current.HasGetter
		}
		if hasSetter {
			newDesc.Setter = setter
			newDesc.HasSetter = true
		} else if current.IsAccessor {
			newDesc.Setter = current.Setter
			newDesc.HasSetter = current.HasSetter
		}
		if wasLiveMapped {
			// ES 10.4.4.7 step 6a: converting a mapped index to an accessor
			// severs the binding entirely.
			newDesc.Unmapped = true
		}
	} else {
		newDesc.Value = current.Value
		if hasValue {
			newDesc.Value = value
		}
		newDesc.Writable = writable
		if wasLiveMapped {
			if hasValue {
				// Step 6b(i): write the new value through to the parameter.
				a.mappedRegs[index] = value
			}
			if writablePtr != nil && !*writablePtr {
				// Step 6b(ii): making it non-writable severs the mapping.
				newDesc.Unmapped = true
				newDesc.Value = a.mappedRegs[index]
			} else {
				newDesc.Unmapped = false
			}
		}
	}

	if a.argDescs == nil {
		a.argDescs = make(map[string]*ArgDescriptor)
	}
	a.argDescs[key] = newDesc
	if a.deletedProps != nil {
		delete(a.deletedProps, key)
	}
	return nil
}

// argDescIfPresent returns the raw override for key without synthesizing
// defaults, or nil if none is recorded.
func (a *ArgumentsObject) argDescIfPresent(key string) *ArgDescriptor {
	if a.argDescs == nil {
		return nil
	}
	return a.argDescs[key]
}

// argumentsDelete implements [[Delete]] for a key on an arguments object:
// returns false (without deleting) if the property exists and is
// non-configurable, matching ES OrdinaryDelete via Reflect.deleteProperty /
// the `delete` operator's semantics. Deleting a still-mapped index also
// severs its binding, per the note in ES 10.4.4.7 that a deleted mapped
// argument's map entry is removed.
func (a *ArgumentsObject) argumentsDelete(key string) bool {
	own := a.ArgumentsOwnProperty(key)
	if !own.Exists {
		return true
	}
	if !own.Configurable {
		return false
	}
	if a.deletedProps == nil {
		a.deletedProps = make(map[string]bool)
	}
	a.deletedProps[key] = true
	if a.argDescs != nil {
		delete(a.argDescs, key)
	}
	return true
}

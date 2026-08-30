package vm

// Functions, RegExps, Maps and Sets are ordinary objects to user code, but the
// VM gives each of them its own Go representation rather than a PlainObject.
// Their *ordinary* own properties therefore live in a side table: a PlainObject
// hanging off a `Properties` field, created lazily the first time anything
// defines a property on the value.
//
// That table is also where per-object state that has nowhere else to live gets
// recorded - notably `extensible`, which is what Object.preventExtensions /
// Object.freeze / Object.seal set and Object.isExtensible / isFrozen / isSealed
// read. Because of that, code asking "what is this value's integrity level?"
// has to be able to *create* the table, not just read it (issue #123): a
// brand-new function has no table at all, and `Object.freeze(function () {})`
// still has to stick.

// newPropertiesTable allocates a side table.
//
// Every lazy-creation site must go through this. The two invariants it carries
// are easy to get wrong by hand, and both were, at different sites:
//
//   - extensible: true. A table born non-extensible reports a brand-new
//     function as sealed the moment Object.isExtensible starts reading it.
//   - prototype: Undefined. The table is a property bag, not the value's
//     [[Prototype]]; an exotic value's real prototype is decided by its kind
//     (Function.prototype, RegExp.prototype, ...). The symbol-lookup walks
//     treat an Undefined table prototype as "keep going", and short-circuit
//     into an empty dead end if it is set to anything else.
func newPropertiesTable() *PlainObject {
	return &PlainObject{prototype: Undefined, shape: RootShape, extensible: true}
}

// OwnPropertiesTable returns the side table holding v's ordinary own
// properties, or nil when v either isn't one of the exotic kinds that use one
// or hasn't needed a table yet. Read-only callers (Object.isExtensible,
// isFrozen, isSealed) want this: a value with no table has never had its
// integrity level touched.
func OwnPropertiesTable(v Value) *PlainObject {
	if slot := ownPropertiesSlot(v); slot != nil {
		return *slot
	}
	return nil
}

// EnsureOwnPropertiesTable is OwnPropertiesTable, but allocates the table if v
// doesn't have one yet. Callers that *record* something (Object.freeze,
// Object.seal, Object.preventExtensions, property definition) want this. Still
// returns nil for values that have no side table at all.
func EnsureOwnPropertiesTable(v Value) *PlainObject {
	slot := ownPropertiesSlot(v)
	if slot == nil {
		return nil
	}
	if *slot == nil {
		*slot = newPropertiesTable()
	}
	return *slot
}

// ownPropertiesSlot returns a pointer to v's Properties field, or nil if v has
// no side table. Returning the slot rather than the value is what lets
// EnsureOwnPropertiesTable install a fresh table.
func ownPropertiesSlot(v Value) **PlainObject {
	switch v.Type() {
	case TypeFunction:
		if fn := v.AsFunction(); fn != nil {
			return &fn.Properties
		}
	case TypeClosure:
		if cl := v.AsClosure(); cl != nil {
			return &cl.Properties
		}
	case TypeNativeFunction:
		if nf := v.AsNativeFunction(); nf != nil {
			return &nf.Properties
		}
	case TypeNativeFunctionWithProps:
		if nfp := v.AsNativeFunctionWithProps(); nfp != nil {
			return &nfp.Properties
		}
	case TypeBoundFunction:
		if bf := v.AsBoundFunction(); bf != nil {
			return &bf.Properties
		}
	case TypeRegExp:
		if re := v.AsRegExpObject(); re != nil {
			return &re.Properties
		}
	case TypeMap:
		if m := v.AsMap(); m != nil {
			return &m.Properties
		}
	case TypeSet:
		if s := v.AsSet(); s != nil {
			return &s.Properties
		}
	}
	return nil
}

// setOwnChecked stores an ordinary own property on a side table, applying the
// two [[Set]] rejections the table can now express (issue #123):
//
//   - the property exists as a non-writable data property, or
//   - the property is new and the table is non-extensible.
//
// A rejection is silent in sloppy mode and a TypeError in strict mode, which is
// what makes Object.freeze / Object.seal / Object.preventExtensions on a
// function, RegExp, Map or Set actually bite. Accessor properties are left to
// the callers that dispatch to setters before they get here.
//
// Returns the opcode-shaped triple, so call sites can `return` it directly.
func (vm *VM) setOwnChecked(props *PlainObject, name string, v Value) (bool, InterpretResult, Value) {
	if allowed, threw := vm.tableSetAllowed(props, keyFromString(name), name); !allowed {
		if threw {
			return false, InterpretRuntimeError, Undefined
		}
		return true, InterpretOK, v
	}
	props.SetOwn(name, v)
	return true, InterpretOK, v
}

// setOwnCheckedByKey is setOwnChecked for a symbol (or otherwise non-string)
// key. It stores through DefineOwnPropertyByKey because that is what the
// symbol-set paths already did.
func (vm *VM) setOwnCheckedByKey(props *PlainObject, key PropertyKey, v Value) (bool, InterpretResult, Value) {
	if allowed, threw := vm.tableSetAllowed(props, key, key.debugName()); !allowed {
		if threw {
			return false, InterpretRuntimeError, Undefined
		}
		return true, InterpretOK, v
	}
	props.DefineOwnPropertyByKey(key, v, nil, nil, nil)
	return true, InterpretOK, v
}

// tableSetAllowed applies setOwnChecked's rejections. threw is true when a
// TypeError has been thrown (strict mode) and the caller must abort.
func (vm *VM) tableSetAllowed(props *PlainObject, key PropertyKey, display string) (allowed, threw bool) {
	if _, _, _, _, isAccessor := props.GetOwnAccessorByKey(key); isAccessor {
		return true, false
	}
	if _, writable, _, _, exists := props.GetOwnDescriptorByKey(key); exists {
		if writable {
			return true, false
		}
		if vm.IsInStrictMode() {
			vm.ThrowTypeError("Cannot assign to read only property '" + display + "'")
			return false, true
		}
		return false, false
	}
	if props.IsExtensible() {
		return true, false
	}
	if vm.IsInStrictMode() {
		vm.ThrowTypeError("Cannot add property " + display + ", object is not extensible")
		return false, true
	}
	return false, false
}

// MaterializeIntrinsicOwnProperties copies a callable's lazily-synthesized own
// properties - "length", "name" and, for functions that have one, "prototype" -
// into its side table.
//
// Those are ordinary own properties per the spec, created on demand here purely
// as an optimization. SetIntegrityLevel walks a value's own property keys, so
// Object.freeze / Object.seal can only cover them if they exist in the table by
// the time the walk happens; otherwise a frozen function still reports
// `name` as configurable and `prototype` as writable, because the descriptor
// fallbacks synthesize a fresh unfrozen answer. Called by freeze and seal only:
// Object.preventExtensions leaves attributes alone, so it has nothing to cover.
//
// Attributes match what the fallbacks report for an untouched function:
// name/length are {writable: false, enumerable: false, configurable: true} and
// prototype is {writable: true, enumerable: false, configurable: false}.
func MaterializeIntrinsicOwnProperties(vmInst *VM, v Value) {
	props := EnsureOwnPropertiesTable(v)
	if props == nil {
		return
	}

	var name string
	var length int
	var deletedName, deletedLength bool
	var prototype Value
	hasPrototype := false

	switch v.Type() {
	case TypeFunction:
		fn := v.AsFunction()
		name, length = fn.Name, fn.Length
		deletedName, deletedLength = fn.DeletedName, fn.DeletedLength
		if !fn.IsArrowFunction {
			prototype, hasPrototype = fn.GetOrCreatePrototypeWithVM(vmInst), true
		}
	case TypeClosure:
		cl := v.AsClosure()
		fn := cl.Fn
		name, length = fn.Name, fn.Length
		deletedName, deletedLength = fn.DeletedName, fn.DeletedLength
		// Same own-property test the descriptor fallback and
		// getOwnPropertyNames use: everything but an arrow has a "prototype".
		// (Methods shouldn't, but this runtime doesn't distinguish them yet and
		// already reports one for them; matching that keeps freeze consistent
		// with what the rest of the runtime says the own properties are.)
		if !fn.IsArrowFunction {
			prototype, hasPrototype = cl.GetPrototypeWithVM(vmInst), true
		}
	case TypeNativeFunction:
		nf := v.AsNativeFunction()
		name, length = nf.Name, nf.Arity
		deletedName, deletedLength = nf.DeletedName, nf.DeletedLength
	case TypeNativeFunctionWithProps:
		nfp := v.AsNativeFunctionWithProps()
		name, length = nfp.Name, nfp.Arity
		deletedName, deletedLength = nfp.DeletedName, nfp.DeletedLength
	default:
		// Bound functions already carry name/length in their table; RegExps,
		// Maps and Sets have no synthesized own properties.
		return
	}

	no, yes := false, true
	if !deletedName {
		if _, ok := props.GetOwn("name"); !ok {
			props.DefineOwnProperty("name", NewString(name), &no, &no, &yes)
		}
	}
	if !deletedLength {
		if _, ok := props.GetOwn("length"); !ok {
			props.DefineOwnProperty("length", NumberValue(float64(length)), &no, &no, &yes)
		}
	}
	if hasPrototype {
		if _, ok := props.GetOwn("prototype"); !ok {
			props.DefineOwnProperty("prototype", prototype, &yes, &no, &no)
		}
	}
}

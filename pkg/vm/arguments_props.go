package vm

import (
	"sort"
	"strconv"
	"unsafe"
)

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
	if index < 0 || index >= a.numMapped || a.mapped == nil {
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
// CreateMappedArgumentsObject; "length" and "callee" get their creation
// attributes unless redefined.
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
			value = a.mappedGet(index)
		}
		return ArgOwnResult{
			Exists: true, Value: value,
			Writable: override.Writable, Enumerable: override.Enumerable, Configurable: override.Configurable,
		}
	}

	// No override recorded: synthesize the default descriptor for whichever
	// kind of key this is. length and callee are ordinary own properties
	// created by CreateMapped/UnmappedArgumentsObject: length {writable,
	// configurable}; callee likewise in sloppy mode, and a
	// %ThrowTypeError% accessor {!enumerable, !configurable} in strict mode.
	switch key {
	case "length":
		v := NumberValue(float64(a.length))
		if nv, ok := a.namedProps[key]; ok {
			v = nv
		}
		return ArgOwnResult{Exists: true, Value: v, Writable: true, Configurable: true}
	case "callee":
		if a.isStrict {
			return ArgOwnResult{Exists: true, IsAccessor: true, Getter: a.thrower, Setter: a.thrower, HasGetter: true, HasSetter: true}
		}
		v := a.callee
		if nv, ok := a.namedProps[key]; ok {
			v = nv
		}
		return ArgOwnResult{Exists: true, Value: v, Writable: true, Configurable: true}
	}
	if isIndex {
		if index < a.numMapped && a.mapped != nil {
			return ArgOwnResult{Exists: true, Value: a.mappedGet(index), Writable: true, Enumerable: true, Configurable: true}
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
	_, err := vm.argumentsSetReport(a, key, value, strict)
	return err
}

// ArgumentsSet is [[Set]] of a string key on an arguments object for native
// callers (Reflect.set, Object.assign): ok is [[Set]]'s boolean result, and a
// rejected write is reported as !ok rather than an error.
func (vm *VM) ArgumentsSet(a *ArgumentsObject, key string, value Value) (bool, error) {
	return vm.argumentsSetReport(a, key, value, false)
}

// argumentsSetReport is argumentsSet that also reports [[Set]]'s result.
func (vm *VM) argumentsSetReport(a *ArgumentsObject, key string, value Value, strict bool) (bool, error) {
	own := a.ArgumentsOwnProperty(key)
	if !own.Exists {
		// A brand-new own property (including an index that was deleted or
		// is past the end): refused on a non-extensible object; otherwise a
		// fresh ordinary data property - an index created this way is never
		// mapped to a parameter (paserati#535).
		if a.nonExtensible {
			if strict {
				return false, vm.NewTypeError("Cannot add property " + key + ", object is not extensible")
			}
			return false, nil
		}
		if _, isIndex := tryParseArgIndex(key); isIndex {
			a.defineFreshIndex(key, value)
		} else {
			a.SetNamedProp(key, value)
		}
		return true, nil
	}

	if own.Exists && own.IsAccessor {
		if own.HasSetter && own.Setter.Type() != TypeUndefined {
			_, err := vm.Call(own.Setter, NewValueFromArguments(a), []Value{value})
			return err == nil, err
		}
		if strict {
			return false, vm.NewTypeError("Cannot set property " + key + " of arguments which has only a getter")
		}
		return false, nil
	}

	if own.Exists && !own.Writable {
		if strict {
			return false, vm.NewTypeError("Cannot assign to read only property '" + key + "' of object '[object Arguments]'")
		}
		return false, nil
	}

	index, isIndex := tryParseArgIndex(key)
	var override *ArgDescriptor
	if a.argDescs != nil {
		override = a.argDescs[key]
	}

	if isIndex && a.isLiveMappedIndex(index, override) {
		a.mappedSet(index, value)
		// Keep a same override's stored value in sync too, purely so a
		// later read that (incorrectly) bypassed isLiveMappedIndex still
		// sees something sane; ArgumentsOwnProperty itself always prefers
		// the live register while mapped.
		if override != nil {
			override.Value = value
		}
		return true, nil
	}

	if override != nil {
		override.Value = value
		return true, nil
	}

	if isIndex {
		a.SetIndexed(index, value)
		return true, nil
	}
	a.SetNamedProp(key, value)
	return true, nil
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
		if a.nonExtensible {
			return vm.NewTypeError("Cannot define property " + key + ", object is not extensible")
		}
		if a.argDescs == nil {
			a.argDescs = make(map[string]*ArgDescriptor)
		}
		a.noteNamedKey(key)
	}

	// Merge onto the resolved current state - missing fields are preserved,
	// matching OrdinaryDefineOwnProperty's "fields not present in Desc keep
	// the current value" rule (defaulting to the ES defaults when the
	// property is being created for the first time).
	// A property created here takes false for every absent field
	// (ValidateAndApplyPropertyDescriptor step 2); this used to default them
	// to true, so defineProperty(arguments, "y", {value: 5}) made y writable,
	// enumerable and configurable (paserati#535).
	writable := current.Writable
	if writablePtr != nil {
		writable = *writablePtr
	}
	enumerable := current.Enumerable
	if enumerablePtr != nil {
		enumerable = *enumerablePtr
	}
	configurable := current.Configurable
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
			newDesc.Unmapped = !(index < a.numMapped && a.mapped != nil)
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
				a.mappedSet(index, value)
			}
			if writablePtr != nil && !*writablePtr {
				// Step 6b(ii): making it non-writable severs the mapping.
				newDesc.Unmapped = true
				newDesc.Value = a.mappedGet(index)
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

// Delete is the exported [[Delete]] for a string key on an arguments object
// (see argumentsDelete), for builtins such as Reflect.deleteProperty that
// need the same semantics as the `delete` operator without going through
// the OpDeleteIndex opcode.
func (a *ArgumentsObject) Delete(key string) bool {
	return a.argumentsDelete(key)
}

// mappedGet / mappedSet read and write a mapped parameter through its
// upvalue: the live register while the function's frame is alive, the
// closed-over value after it returns. Mapping via upvalues rather than a raw
// slice of the frame's registers is what keeps an arguments object that
// outlives its call correct - the registers are reused by later calls, so
// the old raw slice read whatever landed there (paserati#535) - and keeps
// arguments[i] linked to a closure that captured the same parameter, since
// captureUpvalue shares one Upvalue per register.
func (a *ArgumentsObject) mappedGet(index int) Value {
	uv := a.mapped[index]
	if uv.Location != nil {
		return *uv.Location
	}
	return uv.Closed
}

func (a *ArgumentsObject) mappedSet(index int, v Value) {
	uv := a.mapped[index]
	if uv.Location != nil {
		*uv.Location = v
		return
	}
	uv.Closed = v
}

// noteNamedKey records a newly created non-index key other than length and
// callee, for [[OwnPropertyKeys]]'s creation order.
func (a *ArgumentsObject) noteNamedKey(key string) {
	if _, isIndex := tryParseArgIndex(key); isIndex || key == "length" || key == "callee" {
		return
	}
	for _, k := range a.named {
		if k == key {
			return
		}
	}
	a.named = append(a.named, key)
}

// defineFreshIndex creates an index key that is not currently an own
// property as an ordinary, unmapped data property.
func (a *ArgumentsObject) defineFreshIndex(key string, value Value) {
	if a.argDescs == nil {
		a.argDescs = make(map[string]*ArgDescriptor)
	}
	a.argDescs[key] = &ArgDescriptor{Value: value, Writable: true, Enumerable: true, Configurable: true, Unmapped: true}
	if a.deletedProps != nil {
		delete(a.deletedProps, key)
	}
}

// OwnKeys returns the arguments object's own string keys in
// [[OwnPropertyKeys]] order: integer indices ascending, then the remaining
// keys in creation order (length and callee first, as the object is created
// with them).
func (a *ArgumentsObject) OwnKeys() []string {
	seen := map[int]bool{}
	var idx []int
	addIdx := func(k string) {
		if i, ok := tryParseArgIndex(k); ok && !seen[i] {
			seen[i] = true
			idx = append(idx, i)
		}
	}
	for i := 0; i < len(a.args); i++ {
		addIdx(strconv.Itoa(i))
	}
	for k := range a.namedProps {
		addIdx(k)
	}
	for k := range a.argDescs {
		addIdx(k)
	}
	sort.Ints(idx)
	out := make([]string, 0, len(idx)+2+len(a.named))
	for _, i := range idx {
		if k := strconv.Itoa(i); a.ArgumentsOwnProperty(k).Exists {
			out = append(out, k)
		}
	}
	for _, k := range append([]string{"length", "callee"}, a.named...) {
		if a.ArgumentsOwnProperty(k).Exists {
			out = append(out, k)
		}
	}
	return out
}

// PreventExtensions makes the arguments object non-extensible.
func (a *ArgumentsObject) PreventExtensions() { a.nonExtensible = true }

// IsExtensible reports [[IsExtensible]].
func (a *ArgumentsObject) IsExtensible() bool { return !a.nonExtensible }

// LockSymbols makes every own symbol property non-configurable, and also
// non-writable when frozen (Object.seal / Object.freeze).
func (a *ArgumentsObject) LockSymbols(frozen bool) {
	if a.symLocked == nil {
		a.symLocked = make(map[*SymbolObject]uint8)
	}
	level := uint8(1)
	if frozen {
		level = 2
	}
	for sym := range a.symbolProps {
		if a.symLocked[sym] < level {
			a.symLocked[sym] = level
		}
	}
}

// SetThrower installs the realm's %ThrowTypeError%, used by strict-mode
// arguments' callee accessor.
func (a *ArgumentsObject) SetThrower(f Value) { a.thrower = f }

// newArguments is NewArguments plus the realm-dependent parts: the
// %ThrowTypeError% for a strict arguments object's callee accessor.
func (vm *VM) newArguments(args []Value, callee Value, isStrict bool) Value {
	v := NewArguments(args, callee, isStrict)
	v.AsArguments().SetThrower(vm.ThrowTypeErrorFunc)
	return v
}

// SetSymbolChecked is ordinary [[Set]] of an own symbol property: refused
// (false) for a frozen existing property or a new one on a non-extensible
// object.
func (a *ArgumentsObject) SetSymbolChecked(sym *SymbolObject, v Value) bool {
	if _, exists := a.symbolProps[sym]; exists {
		if a.symLocked[sym] >= 2 {
			return false
		}
	} else if a.nonExtensible {
		return false
	}
	a.SetSymbolProp(sym, v)
	return true
}

// argumentsGetProp is [[Get]] of a string key on an arguments object for the
// property-access opcodes: an own data property's value, an own accessor's
// getter (strict callee's %ThrowTypeError% throws), otherwise the
// [[Prototype]] chain.
func (vm *VM) argumentsGetProp(frame *CallFrame, ip int, frameWasNil bool, base Value, key string, dest *Value) (bool, InterpretResult, Value) {
	own := base.AsArguments().ArgumentsOwnProperty(key)
	if own.Exists {
		if own.IsAccessor {
			getter := Undefined
			if own.HasGetter {
				getter = own.Getter
			}
			return vm.invokeSymbolGetter(frame, ip, frameWasNil, getter, base, dest)
		}
		*dest = own.Value
		return true, InterpretOK, *dest
	}
	return vm.finishProtoChainGet(frame, ip, frameWasNil, key, base, dest)
}

// OwnSymbolKeyValues returns the own symbol keys, in creation order, as
// symbol Values.
func (a *ArgumentsObject) OwnSymbolKeyValues() []Value {
	syms := a.OwnSymbolKeys()
	out := make([]Value, len(syms))
	for i, sym := range syms {
		out[i] = Value{typ: TypeSymbol, obj: unsafe.Pointer(sym)}
	}
	return out
}

// ArgumentsHasProperty is [[HasProperty]] on an arguments object: an own
// property (per ArgumentsOwnProperty, or an own symbol property), else the
// [[Prototype]] chain.
func (vm *VM) ArgumentsHasProperty(v Value, key PropertyKey) bool {
	a := v.AsArguments()
	if key.isSymbol() {
		if sym := key.symbolVal.AsSymbolObject(); sym != nil && a.HasOwnSymbolProp(sym) {
			return true
		}
	} else if a.ArgumentsOwnProperty(key.name).Exists {
		return true
	}
	return vm.hasPropertyByKeyFromPrototypeChain(vm.PrototypeOf(v), key)
}

// ArgumentsSetIntegrity implements SetIntegrityLevel (Object.seal /
// Object.freeze) for an arguments object: every own property becomes
// non-configurable, data properties also non-writable when frozen, and the
// object non-extensible. Freezing a still-mapped index severs its mapping
// (10.4.4.7 step 6.b.ii).
func (vm *VM) ArgumentsSetIntegrity(a *ArgumentsObject, frozen bool) error {
	f := false
	for _, k := range a.OwnKeys() {
		own := a.ArgumentsOwnProperty(k)
		var wp *bool
		if frozen && !own.IsAccessor {
			wp = &f
		}
		if err := vm.ArgumentsDefineOwnProperty(a, k, false, Undefined, wp, nil, &f, false, Undefined, false, Undefined); err != nil {
			return err
		}
	}
	a.LockSymbols(frozen)
	a.PreventExtensions()
	return nil
}

// ArgumentsTestIntegrity implements TestIntegrityLevel (Object.isSealed /
// Object.isFrozen) for an arguments object.
func (a *ArgumentsObject) ArgumentsTestIntegrity(frozen bool) bool {
	if !a.nonExtensible {
		return false
	}
	for _, k := range a.OwnKeys() {
		own := a.ArgumentsOwnProperty(k)
		if own.Configurable || (frozen && !own.IsAccessor && own.Writable) {
			return false
		}
	}
	need := uint8(1)
	if frozen {
		need = 2
	}
	for sym := range a.symbolProps {
		if a.symLocked[sym] < need {
			return false
		}
	}
	return true
}

// copyArgumentsDataProperties is CopyDataProperties (7.3.26) from an
// arguments object into a plain or dict object.
func (vm *VM) copyArgumentsDataProperties(dest Value, source Value) error {
	a := source.AsArguments()
	w, e, c := true, true, true
	for _, k := range a.OwnKeys() {
		if !a.ArgumentsOwnProperty(k).Enumerable {
			continue
		}
		v, err := vm.argumentsGet(a, k)
		if err != nil {
			return err
		}
		if dest.Type() == TypeDictObject {
			dest.AsDictObject().SetOwn(k, v)
		} else {
			dest.AsPlainObject().DefineOwnPropertyByKey(keyFromString(k), v, &w, &e, &c)
		}
	}
	if dest.Type() != TypeObject {
		return nil
	}
	for _, sv := range a.OwnSymbolKeyValues() {
		sym := sv.AsSymbolObject()
		if _, en, _ := a.SymbolPropAttrs(sym); !en {
			continue
		}
		v, _ := a.GetSymbolProp(sym)
		dest.AsPlainObject().DefineOwnPropertyByKey(NewSymbolKey(sv), v, &w, &e, &c)
	}
	return nil
}

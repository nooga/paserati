package builtins

import (
	"math"
	"sort"
	"strconv"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// ObjectInitializer implements the Object builtin
type ObjectInitializer struct{}

func (o *ObjectInitializer) Name() string {
	return "Object"
}

func (o *ObjectInitializer) Priority() int {
	return PriorityObject // Must be first (base prototype)
}

func (o *ObjectInitializer) InitTypes(ctx *TypeContext) error {
	// Create Object.prototype type using fluent API
	// For property key parameters, accept string | symbol
	keyStringOrSymbol := types.NewUnionType(types.String, types.Symbol)
	objectProtoType := types.NewObjectType().
		WithProperty("hasOwnProperty", types.NewSimpleFunction([]types.Type{keyStringOrSymbol}, types.Boolean)).
		WithProperty("propertyIsEnumerable", types.NewSimpleFunction([]types.Type{keyStringOrSymbol}, types.Boolean)).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toLocaleString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("isPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean))

	// Create Object constructor type using fluent API
	objectCtorType := types.NewObjectType().
		// Constructor is callable with optional parameter
		WithSimpleCallSignature([]types.Type{}, types.Any).
		WithSimpleCallSignature([]types.Type{types.Any}, types.Any).
		// Static methods
		WithProperty("create", types.NewOptionalFunction(
			[]types.Type{types.Any, types.Any},
			types.Any,
			[]bool{false, true}, // First param required, second optional
		)).
		WithProperty("keys", types.NewSimpleFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: types.String})).
		WithProperty("values", types.NewSimpleFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: types.Any})).
		WithProperty("entries", types.NewSimpleFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: &types.TupleType{ElementTypes: []types.Type{types.String, types.Any}}})).
		WithProperty("getOwnPropertyNames", types.NewSimpleFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: types.String})).
		WithProperty("getOwnPropertySymbols", types.NewSimpleFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: types.Symbol})).
		WithProperty("assign", types.NewVariadicFunction([]types.Type{types.Any}, types.Any, &types.ArrayType{ElementType: types.Any})).
		WithProperty("hasOwn", types.NewSimpleFunction([]types.Type{types.Any, keyStringOrSymbol}, types.Boolean)).
		WithProperty("fromEntries", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("getPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("setPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Any)).
		WithProperty("defineProperty", types.NewSimpleFunction([]types.Type{types.Any, keyStringOrSymbol, types.Any}, types.Any)).
		WithProperty("defineProperties", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Any)).
		WithProperty("getOwnPropertyDescriptor", types.NewSimpleFunction([]types.Type{types.Any, keyStringOrSymbol}, types.Any)).
		WithProperty("getOwnPropertyDescriptors", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("isExtensible", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("preventExtensions", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("freeze", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("seal", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("isFrozen", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("isSealed", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("is", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Boolean)).
		// Object.groupBy(items, callbackfn)
		WithProperty("groupBy", types.NewSimpleFunction(
			[]types.Type{
				types.Any, // items: Iterable<T>
				types.NewSimpleFunction([]types.Type{types.Any, types.Number}, keyStringOrSymbol), // callbackfn: (value: T, index: number) => PropertyKey
			},
			types.NewObjectType(), // Record<PropertyKey, T[]>
		)).
		WithProperty("prototype", objectProtoType)

	// Define the constructor globally
	if err := ctx.DefineGlobal("Object", objectCtorType); err != nil {
		return err
	}

	// Store the prototype type for primitive "object"
	ctx.SetPrimitivePrototype("object", objectProtoType)

	return nil
}

func (o *ObjectInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Object.prototype - this is the root prototype (no parent)
	objectProto := vm.NewObject(vm.Null).AsPlainObject()

	// Add prototype methods
	objectProto.SetOwnNonEnumerable("hasOwnProperty", vm.NewNativeFunction(1, false, "hasOwnProperty", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		// Step 1: Let P be ? ToPropertyKey(V). (BEFORE ToObject per spec)
		keyVal := args[0]
		if keyVal.IsObject() || keyVal.IsCallable() {
			resolved, err := toPropertyKeyValue(vmInstance, keyVal)
			if err != nil {
				return vm.Undefined, err
			}
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			keyVal = resolved
		}
		// Step 2: Let O be ? ToObject(this value). Throws TypeError for null/undefined.
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeNull || thisValue.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		// Handle symbol and string keys
		if keyVal.Type() == vm.TypeSymbol {
			key := vm.NewSymbolKey(keyVal)
			// IMPORTANT: Check type BEFORE calling As*() methods since they panic on type mismatch
			switch thisValue.Type() {
			case vm.TypeObject:
				return vm.BooleanValue(thisValue.AsPlainObject().HasOwnByKey(key)), nil
			case vm.TypeDictObject:
				// DictObject has only string keys; symbols are not supported
				return vm.BooleanValue(false), nil
			case vm.TypeArray:
				// Arrays: symbol own keys generally none here
				return vm.BooleanValue(false), nil
			case vm.TypeArguments:
				// Arguments objects have own symbol properties (e.g., Symbol.iterator)
				return vm.BooleanValue(thisValue.AsArguments().HasOwnSymbolProp(keyVal.AsSymbolObject())), nil
			case vm.TypeProxy:
				// Per spec, hasOwnProperty uses [[GetOwnProperty]] which goes through the proxy protocol
				desc, err := objectGetOwnPropertyDescriptorWithVM(vmInstance, []vm.Value{thisValue, keyVal})
				if err != nil {
					return vm.Undefined, err
				}
				return vm.BooleanValue(!desc.IsUndefined()), nil
			case vm.TypeNativeFunctionWithProps:
				nfp := thisValue.AsNativeFunctionWithProps()
				if nfp.Properties != nil {
					return vm.BooleanValue(nfp.Properties.HasOwnByKey(key)), nil
				}
				return vm.BooleanValue(false), nil
			case vm.TypeFunction:
				fn := thisValue.AsFunction()
				if fn.Properties != nil {
					return vm.BooleanValue(fn.Properties.HasOwnByKey(key)), nil
				}
				return vm.BooleanValue(false), nil
			case vm.TypeClosure:
				cl := thisValue.AsClosure()
				if cl.Properties != nil {
					if cl.Properties.HasOwnByKey(key) {
						return vm.BooleanValue(true), nil
					}
				}
				if cl.Fn != nil && cl.Fn.Properties != nil {
					return vm.BooleanValue(cl.Fn.Properties.HasOwnByKey(key)), nil
				}
				return vm.BooleanValue(false), nil
			default:
				return vm.BooleanValue(false), nil
			}
		}
		propName := keyVal.ToString()

		// Check if this object has the property as own property
		// IMPORTANT: Check type BEFORE calling As*() methods since they panic on type mismatch
		switch thisValue.Type() {
		case vm.TypeObject:
			plainObj := thisValue.AsPlainObject()
			// Special case for globalThis: check heap for top-level declarations
			if plainObj == vmInstance.GlobalObject {
				if idx, exists := vmInstance.GetHeap().GetNameToIndex()[propName]; exists {
					// Also verify the heap value is still initialized (not deleted)
					if _, isInit := vmInstance.GetHeap().Get(idx); isInit {
						return vm.BooleanValue(true), nil
					}
				}
			}
			_, hasOwn := plainObj.GetOwn(propName)
			return vm.BooleanValue(hasOwn), nil
		case vm.TypeDictObject:
			dictObj := thisValue.AsDictObject()
			_, hasOwn := dictObj.GetOwn(propName)
			return vm.BooleanValue(hasOwn), nil
		case vm.TypeArray:
			arrObj := thisValue.AsArray()
			// For arrays, check if it's a valid index or 'length'
			if propName == "length" {
				return vm.BooleanValue(true), nil
			}
			// Check numeric indices. An own accessor at this index (set via
			// Object.defineProperty - see ArrayDefineOwnProperty) counts
			// even for an index >= Length() (huge sparse indices are
			// tracked in the properties map, not elements - see
			// maxDenseArrayDefineIndex); otherwise fall back to
			// HasIndex, which correctly reports a hole (from a literal
			// array's elision, or a prior `delete arr[i]`) as absent
			// rather than just checking the index is in bounds.
			if index, err := strconv.Atoi(propName); err == nil && index >= 0 {
				return vm.BooleanValue(arrObj.HasOwnIndexProperty(propName, index)), nil
			}
			// Check custom named properties (e.g., pos, end for TypeScript node arrays)
			_, hasOwn := arrObj.GetOwn(propName)
			return vm.BooleanValue(hasOwn), nil
		case vm.TypeFunction:
			fn := thisValue.AsFunction()
			// Functions have intrinsic own properties: name, length
			// Check if they've been deleted (configurable:true means they can be deleted)
			if propName == "name" && !fn.DeletedName {
				return vm.BooleanValue(true), nil
			}
			if propName == "length" && !fn.DeletedLength {
				return vm.BooleanValue(true), nil
			}
			// Per ECMAScript spec, all non-arrow functions have "prototype" as an own property.
			// It is created lazily but should always be reported as present.
			if propName == "prototype" && !fn.IsArrowFunction {
				return vm.BooleanValue(true), nil
			}
			// Check custom properties
			if fn.Properties != nil {
				_, hasOwn := fn.Properties.GetOwn(propName)
				return vm.BooleanValue(hasOwn), nil
			}
			return vm.BooleanValue(false), nil
		case vm.TypeClosure:
			closure := thisValue.AsClosure()
			// Closures have intrinsic own properties: name, length
			// Check if they've been deleted (configurable:true means they can be deleted)
			if propName == "name" && !closure.Fn.DeletedName {
				return vm.BooleanValue(true), nil
			}
			if propName == "length" && !closure.Fn.DeletedLength {
				return vm.BooleanValue(true), nil
			}
			// prototype is only an own property for non-arrow functions that are NOT methods
			// Arrow functions and methods don't have prototype
			// We check if it exists in Properties (created lazily or explicitly)
			// or if the function is a class constructor (always has prototype)
			if propName == "prototype" && !closure.Fn.IsArrowFunction {
				// Class constructors always have prototype
				if closure.Fn.IsClassConstructor {
					return vm.BooleanValue(true), nil
				}
				// Generator functions (sync and async) always have prototype per spec
				if closure.Fn.IsGenerator {
					return vm.BooleanValue(true), nil
				}
				// For closures, check both closure.Properties and Fn.Properties
				if closure.Properties != nil {
					if _, hasOwn := closure.Properties.GetOwn("prototype"); hasOwn {
						return vm.BooleanValue(true), nil
					}
				}
				if closure.Fn.Properties != nil {
					if _, hasOwn := closure.Fn.Properties.GetOwn("prototype"); hasOwn {
						return vm.BooleanValue(true), nil
					}
				}
			}
			// Check closure's own properties first
			if closure.Properties != nil {
				if _, hasOwn := closure.Properties.GetOwn(propName); hasOwn {
					return vm.BooleanValue(true), nil
				}
			}
			// Then check underlying function's properties
			if closure.Fn.Properties != nil {
				_, hasOwn := closure.Fn.Properties.GetOwn(propName)
				return vm.BooleanValue(hasOwn), nil
			}
			return vm.BooleanValue(false), nil
		case vm.TypeNativeFunction:
			nf := thisValue.AsNativeFunction()
			if propName == "name" && !nf.DeletedName {
				return vm.BooleanValue(true), nil
			}
			if propName == "length" && !nf.DeletedLength {
				return vm.BooleanValue(true), nil
			}
			if nf.Properties != nil {
				_, hasOwn := nf.Properties.GetOwn(propName)
				return vm.BooleanValue(hasOwn), nil
			}
			return vm.BooleanValue(false), nil
		case vm.TypeNativeFunctionWithProps:
			nfp := thisValue.AsNativeFunctionWithProps()
			if propName == "name" && !nfp.DeletedName {
				return vm.BooleanValue(true), nil
			}
			if propName == "length" && !nfp.DeletedLength {
				return vm.BooleanValue(true), nil
			}
			if nfp.Properties != nil {
				_, hasOwn := nfp.Properties.GetOwn(propName)
				return vm.BooleanValue(hasOwn), nil
			}
			return vm.BooleanValue(false), nil
		case vm.TypeBoundFunction:
			// Bound functions: name/length are real own properties stored in Properties
			// They can be deleted (configurable:true), so check Properties directly
			bf := thisValue.AsBoundFunction()
			if bf.Properties != nil {
				_, hasOwn := bf.Properties.GetOwn(propName)
				return vm.BooleanValue(hasOwn), nil
			}
			return vm.BooleanValue(false), nil
		case vm.TypeArguments:
			argsObj := thisValue.AsArguments()
			// Arguments objects have own properties: length, callee, and numeric indices
			if propName == "length" {
				return vm.BooleanValue(true), nil
			}
			if propName == "callee" {
				return vm.BooleanValue(true), nil
			}
			// Check numeric indices
			if index, err := strconv.Atoi(propName); err == nil {
				return vm.BooleanValue(index >= 0 && index < argsObj.Length()), nil
			}
			// Check overflow named properties
			return vm.BooleanValue(argsObj.HasNamedProp(propName)), nil
		case vm.TypeRegExp:
			// RegExp objects have intrinsic own property: lastIndex
			// (source, flags, global, etc. are on prototype in modern JS)
			if propName == "lastIndex" {
				return vm.BooleanValue(true), nil
			}
			// Check custom properties
			regexObj := thisValue.AsRegExpObject()
			if regexObj != nil && regexObj.Properties != nil {
				_, hasOwn := regexObj.Properties.GetOwn(propName)
				return vm.BooleanValue(hasOwn), nil
			}
			return vm.BooleanValue(false), nil
		case vm.TypeProxy:
			// Per spec, hasOwnProperty uses [[GetOwnProperty]] which goes through the proxy protocol
			desc, err := objectGetOwnPropertyDescriptorWithVM(vmInstance, []vm.Value{thisValue, keyVal})
			if err != nil {
				return vm.Undefined, err
			}
			return vm.BooleanValue(!desc.IsUndefined()), nil
		default:
			return vm.BooleanValue(false), nil
		}
	}))
	// Ensure attributes per spec: writable true, enumerable false, configurable true
	if v, ok := objectProto.GetOwn("hasOwnProperty"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("hasOwnProperty", v, &w, &e, &c)
	}

	// propertyIsEnumerable
	objectProto.SetOwnNonEnumerable("propertyIsEnumerable", vm.NewNativeFunction(1, false, "propertyIsEnumerable", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		thisValue := vmInstance.GetThis()
		// Step 1: Let O be ? ToObject(this value). Throws TypeError for null/undefined.
		if thisValue.Type() == vm.TypeNull || thisValue.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		keyVal := args[0]
		// ToPropertyKey — if key is an object, call ToPrimitive("string")
		if keyVal.IsObject() || keyVal.IsCallable() {
			resolved, err := toPropertyKeyValue(vmInstance, keyVal)
			if err != nil {
				return vm.Undefined, err
			}
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			keyVal = resolved
		}
		if keyVal.Type() == vm.TypeSymbol {
			// Guard on Type() before AsX() - AsPlainObject()/AsDictObject()/
			// AsArray() panic instead of returning nil for a mismatched type,
			// so calling AsPlainObject() unconditionally here (as this used
			// to) panicked for every non-plain-object `this`, including
			// arguments objects (which do have own symbol properties, e.g.
			// Symbol.iterator - see language/arguments-object/mapped/
			// Symbol.iterator.js).
			key := vm.NewSymbolKey(keyVal)
			switch thisValue.Type() {
			case vm.TypeObject:
				if _, _, en, _, ok := thisValue.AsPlainObject().GetOwnDescriptorByKey(key); ok {
					return vm.BooleanValue(en), nil
				}
			case vm.TypeArguments:
				// Symbol.iterator is the only own symbol property arguments
				// objects have, and it's non-enumerable per spec.
				if thisValue.AsArguments().HasOwnSymbolProp(keyVal.AsSymbolObject()) {
					return vm.BooleanValue(false), nil
				}
			}
			return vm.BooleanValue(false), nil
		}
		propName := keyVal.ToString()
		// IMPORTANT: Check type BEFORE calling As*() methods since they panic on type mismatch
		switch thisValue.Type() {
		case vm.TypeObject:
			po := thisValue.AsPlainObject()
			if _, _, en, _, ok := po.GetOwnDescriptor(propName); ok {
				return vm.BooleanValue(en), nil
			}
			return vm.BooleanValue(false), nil
		case vm.TypeDictObject:
			dict := thisValue.AsDictObject()
			if _, _, en, _, ok := dict.GetOwnDescriptor(propName); ok {
				return vm.BooleanValue(en), nil
			}
			return vm.BooleanValue(false), nil
		case vm.TypeArray:
			arr := thisValue.AsArray()
			if propName == "length" {
				return vm.BooleanValue(false), nil
			}
			if idx, err := strconv.Atoi(propName); err == nil && idx >= 0 && idx < arr.Length() {
				return vm.BooleanValue(true), nil
			}
			if _, enumerable, ok := arr.GetNamedPropertyDescriptor(propName); ok {
				return vm.BooleanValue(enumerable), nil
			}
			return vm.BooleanValue(false), nil
		case vm.TypeClosure:
			// Closure own properties: name, length are configurable but not enumerable
			// prototype is also not enumerable for generator functions
			if propName == "name" || propName == "length" || propName == "prototype" {
				return vm.BooleanValue(false), nil
			}
			// Check closure.Properties for custom properties
			closure := thisValue.AsClosure()
			if closure.Properties != nil {
				if _, _, en, _, ok := closure.Properties.GetOwnDescriptor(propName); ok {
					return vm.BooleanValue(en), nil
				}
			}
			// Check Fn.Properties
			if closure.Fn.Properties != nil {
				if _, _, en, _, ok := closure.Fn.Properties.GetOwnDescriptor(propName); ok {
					return vm.BooleanValue(en), nil
				}
			}
			return vm.BooleanValue(false), nil
		case vm.TypeFunction:
			// Function own properties: name, length are configurable but not enumerable
			// prototype is also not enumerable
			if propName == "name" || propName == "length" || propName == "prototype" {
				return vm.BooleanValue(false), nil
			}
			fn := thisValue.AsFunction()
			if fn.Properties != nil {
				if _, _, en, _, ok := fn.Properties.GetOwnDescriptor(propName); ok {
					return vm.BooleanValue(en), nil
				}
			}
			return vm.BooleanValue(false), nil
		case vm.TypeArguments:
			argsObj := thisValue.AsArguments()
			// Per spec: length and callee are non-enumerable
			if propName == "length" || propName == "callee" {
				return vm.BooleanValue(false), nil
			}
			// Numeric indices: consult any defineProperty override instead of
			// assuming the CreateMappedArgumentsObject default of true.
			own := argsObj.ArgumentsOwnProperty(propName)
			return vm.BooleanValue(own.Exists && own.Enumerable), nil
		case vm.TypeProxy:
			// Per spec, propertyIsEnumerable uses [[GetOwnProperty]] which goes through the proxy protocol
			desc, err := objectGetOwnPropertyDescriptorWithVM(vmInstance, []vm.Value{thisValue, keyVal})
			if err != nil {
				return vm.Undefined, err
			}
			if desc.IsUndefined() {
				return vm.BooleanValue(false), nil
			}
			if desc.Type() == vm.TypeObject {
				descObj := desc.AsPlainObject()
				if enumVal, hasEnum := descObj.GetOwn("enumerable"); hasEnum {
					return vm.BooleanValue(!enumVal.IsFalsey()), nil
				}
			}
			return vm.BooleanValue(false), nil
		default:
			return vm.BooleanValue(false), nil
		}
	}))
	if v, ok := objectProto.GetOwn("propertyIsEnumerable"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("propertyIsEnumerable", v, &w, &e, &c)
	}

	objectProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()

		// ECMAScript 20.1.3.6 Object.prototype.toString
		// Step 1-2: Handle null and undefined
		switch thisValue.Type() {
		case vm.TypeNull:
			return vm.NewString("[object Null]"), nil
		case vm.TypeUndefined:
			return vm.NewString("[object Undefined]"), nil
		}

		// Step 4: Let isArray be ? IsArray(O).
		isArr, isArrErr := isArrayValueOrError(vmInstance, thisValue)
		if isArrErr != nil {
			return vm.Undefined, isArrErr
		}

		// Steps 5-14: Determine builtinTag
		// Per spec, only these internal slots determine builtinTag:
		// IsArray → "Array", [[ParameterMap]] → "Arguments", [[Call]] → "Function",
		// [[ErrorData]] → "Error", [[BooleanData]] → "Boolean", [[NumberData]] → "Number",
		// [[StringData]] → "String", [[DateValue]] → "Date", [[RegExpMatcher]] → "RegExp"
		// Everything else → "Object"
		var builtinTag string
		if isArr {
			builtinTag = "Array"
		} else {
			switch thisValue.Type() {
			case vm.TypeArguments:
				builtinTag = "Arguments"
			case vm.TypeFunction, vm.TypeNativeFunction, vm.TypeClosure, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction:
				builtinTag = "Function"
			case vm.TypeRegExp:
				builtinTag = "RegExp"
			case vm.TypeObject:
				if thisValue.Type() == vm.TypeObject {
					plainObj := thisValue.AsPlainObject()
					if _, hasErrorData := plainObj.GetOwn("[[ErrorData]]"); hasErrorData {
						builtinTag = "Error"
					} else if primitiveVal, exists := plainObj.GetOwn("[[PrimitiveValue]]"); exists {
						switch primitiveVal.Type() {
						case vm.TypeBoolean:
							builtinTag = "Boolean"
						case vm.TypeFloatNumber, vm.TypeIntegerNumber:
							builtinTag = "Number"
						case vm.TypeString:
							builtinTag = "String"
						default:
							builtinTag = "Object"
						}
					} else {
						builtinTag = "Object"
					}
				} else {
					builtinTag = "Object"
				}
			case vm.TypeProxy:
				// Proxy builtinTag: if callable → "Function", else "Object"
				if thisValue.IsCallable() {
					builtinTag = "Function"
				} else {
					builtinTag = "Object"
				}
			case vm.TypeBoolean:
				builtinTag = "Boolean"
			case vm.TypeFloatNumber, vm.TypeIntegerNumber:
				builtinTag = "Number"
			case vm.TypeString:
				builtinTag = "String"
			default:
				builtinTag = "Object"
			}
		}

		// Step 15-17: Let tag be ? Get(O, @@toStringTag). If not String, use builtinTag.
		tag, tagErr := lookupToStringTag(vmInstance, thisValue)
		if tagErr != nil {
			return vm.Undefined, tagErr
		}
		if tag.Type() == vm.TypeString {
			return vm.NewString("[object " + tag.ToString() + "]"), nil
		}

		return vm.NewString("[object " + builtinTag + "]"), nil
	}))
	if v, ok := objectProto.GetOwn("toString"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("toString", v, &w, &e, &c)
	}

	objectProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		// ECMAScript 20.1.3.7: Let O be ? ToObject(this value). Return O.
		return vmInstance.ToObject(vmInstance.GetThis())
	}))
	if v, ok := objectProto.GetOwn("valueOf"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("valueOf", v, &w, &e, &c)
	}

	objectProto.SetOwnNonEnumerable("toLocaleString", vm.NewNativeFunction(0, false, "toLocaleString", func(args []vm.Value) (vm.Value, error) {
		// ECMAScript 20.1.3.5: Object.prototype.toLocaleString
		// 1. Let O be the this value.
		// 2. Return ? Invoke(O, "toString").
		thisValue := vmInstance.GetThis()

		// Invoke(O, "toString") throws TypeError for null/undefined (can't get property from them)
		if thisValue.IsUndefined() || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot read properties of " + thisValue.ToString() + " (reading 'toString')")
		}

		// Get toString method and call with the original this value (no ToObject wrapping)
		if toStringMethod, err := vmInstance.GetProperty(thisValue, "toString"); err == nil && toStringMethod.IsCallable() {
			return vmInstance.Call(toStringMethod, thisValue, []vm.Value{})
		}
		return vm.NewString("[object Object]"), nil
	}))
	if v, ok := objectProto.GetOwn("toLocaleString"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("toLocaleString", v, &w, &e, &c)
	}

	objectProto.SetOwnNonEnumerable("isPrototypeOf", vm.NewNativeFunction(1, false, "isPrototypeOf", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		obj := args[0]

		// Step 1: If Type(V) is not Object, return false
		if !obj.IsObject() && !obj.IsCallable() {
			return vm.BooleanValue(false), nil
		}

		// Step 2: Let O be ? ToObject(this value). Throws TypeError for null/undefined.
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeNull || thisValue.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}

		// Walk up the prototype chain of obj via [[GetPrototypeOf]]
		// This uses getPrototypeOfValue which handles Proxy getPrototypeOf traps
		for i := 0; i < 1000; i++ {
			proto, err := getPrototypeOfValue(vmInstance, obj)
			if err != nil {
				return vm.Undefined, err
			}
			if proto.Type() == vm.TypeNull || proto.IsUndefined() {
				return vm.BooleanValue(false), nil
			}
			if proto.Is(thisValue) {
				return vm.BooleanValue(true), nil
			}
			obj = proto
		}
		return vm.BooleanValue(false), nil
	}))
	if v, ok := objectProto.GetOwn("isPrototypeOf"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("isPrototypeOf", v, &w, &e, &c)
	}

	// __proto__ accessor property (ES6 B.2.2.1)
	// Getter: B.2.2.1.1 get Object.prototype.__proto__
	protoGetter := vm.NewNativeFunction(0, false, "get __proto__", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeUndefined || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot read property '__proto__' of " + thisValue.TypeName())
		}
		// Use getPrototypeOfValue which handles Proxy traps and all object types
		return getPrototypeOfValue(vmInstance, thisValue)
	})
	// Setter: Object.prototype.__proto__ (B.2.2.1.2)
	protoSetter := vm.NewNativeFunction(1, false, "set __proto__", func(args []vm.Value) (vm.Value, error) {
		// Step 1: Let O be ? RequireObjectCoercible(this value)
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeUndefined || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot set property '__proto__' of " + thisValue.TypeName())
		}
		if len(args) == 0 {
			return vm.Undefined, nil
		}
		protoArg := args[0]
		// Step 2: If Type(proto) is neither Object nor Null, return undefined
		if protoArg.Type() != vm.TypeNull && !protoArg.IsObject() && !protoArg.IsCallable() {
			return vm.Undefined, nil
		}
		// Step 3: If Type(O) is not Object, return undefined
		if !thisValue.IsObject() && !thisValue.IsCallable() {
			return vm.Undefined, nil
		}
		// Step 4-5: Let status be ? O.[[SetPrototypeOf]](proto). If false, throw TypeError.
		// Handle Proxy objects with setPrototypeOf trap
		if thisValue.Type() == vm.TypeProxy {
			proxy := thisValue.AsProxy()
			if proxy.Revoked {
				return vm.Undefined, vmInstance.NewTypeError("Cannot set prototype of revoked Proxy")
			}
			handler := proxy.Handler()
			// GetMethod(handler, "setPrototypeOf") per spec: an inherited
			// trap counts, not just an own one, and a TypeDictObject
			// handler (a TS enum or module namespace value at runtime)
			// must still be checked for the trap instead of being treated
			// as trap-less outright - vmInstance.ProxyGetTrap, not the
			// previous `if handler.Type() == vm.TypeObject { ...GetOwn... }`
			// which silently answered "no trap" for any other handler kind.
			setProtoTrap, hasTrap := vmInstance.ProxyGetTrap(handler, "setPrototypeOf")
			if hasTrap && setProtoTrap.IsCallable() {
				result, err := vmInstance.CallArgs2(setProtoTrap, handler, proxy.Target(), protoArg)
				if err != nil {
					return vm.Undefined, err
				}
				if result.IsFalsey() {
					return vm.Undefined, vmInstance.NewTypeError("'setPrototypeOf' on proxy: trap returned falsish")
				}
				return vm.Undefined, nil
			}
			// No trap, delegate to target
			thisValue = proxy.Target()
		}
		// Set prototype via OrdinarySetPrototypeOf (handles cycle detection + non-extensible)
		success := true
		switch thisValue.Type() {
		case vm.TypeObject:
			if thisValue.Type() == vm.TypeObject {
				po := thisValue.AsPlainObject()
				success = po.SetPrototype(protoArg)
			}
		case vm.TypeDictObject:
			if thisValue.Type() == vm.TypeDictObject {
				d := thisValue.AsDictObject()
				success = d.SetPrototype(protoArg)
			}
		default:
			// Other object types (arrays, functions, etc.) - delegate to objectSetPrototypeOfWithVM
			_, err := objectSetPrototypeOfWithVM(vmInstance, []vm.Value{thisValue, protoArg})
			if err != nil {
				return vm.Undefined, err
			}
			return vm.Undefined, nil
		}
		if !success {
			return vm.Undefined, vmInstance.NewTypeError("Cyclic __proto__ value")
		}
		return vm.Undefined, nil
	})
	// Define as accessor property: enumerable=false, configurable=true
	e, c := false, true
	objectProto.DefineAccessorProperty("__proto__", protoGetter, true, protoSetter, true, &e, &c)

	// __defineGetter__ (ES6 B.2.2.2) - legacy method to define a getter
	// Per spec: 1. ToObject(this), 2. IsCallable check, 3. ToPropertyKey(P), 4. DefinePropertyOrThrow
	defineGetterFunc := vm.NewNativeFunction(2, false, "__defineGetter__", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeUndefined || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("__defineGetter__ requires 2 arguments")
		}
		getter := args[1]
		if !getter.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("__defineGetter__ getter must be a function")
		}
		// Step 4: ToPropertyKey - may call toString() which can throw
		keyVal, err := toPropertyKeyValue(vmInstance, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// Step 5: DefinePropertyOrThrow with {get: getter, enumerable: true, configurable: true}
		descObj := vm.NewObject(vm.Null).AsPlainObject()
		descObj.SetOwn("get", getter)
		descObj.SetOwn("enumerable", vm.True)
		descObj.SetOwn("configurable", vm.True)
		_, err = objectDefinePropertyWithVM(vmInstance, []vm.Value{thisValue, keyVal, vm.NewValueFromPlainObject(descObj)})
		return vm.Undefined, err
	})
	objectProto.SetOwnNonEnumerable("__defineGetter__", defineGetterFunc)

	// __defineSetter__ (ES6 B.2.2.3) - legacy method to define a setter
	defineSetterFunc := vm.NewNativeFunction(2, false, "__defineSetter__", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeUndefined || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("__defineSetter__ requires 2 arguments")
		}
		setter := args[1]
		if !setter.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("__defineSetter__ setter must be a function")
		}
		// Step 4: ToPropertyKey - may call toString() which can throw
		keyVal, err := toPropertyKeyValue(vmInstance, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// Step 5: DefinePropertyOrThrow with {set: setter, enumerable: true, configurable: true}
		descObj := vm.NewObject(vm.Null).AsPlainObject()
		descObj.SetOwn("set", setter)
		descObj.SetOwn("enumerable", vm.True)
		descObj.SetOwn("configurable", vm.True)
		_, err = objectDefinePropertyWithVM(vmInstance, []vm.Value{thisValue, keyVal, vm.NewValueFromPlainObject(descObj)})
		return vm.Undefined, err
	})
	objectProto.SetOwnNonEnumerable("__defineSetter__", defineSetterFunc)

	// __lookupGetter__ (ES6 B.2.2.4) - legacy method to lookup a getter
	lookupGetterFunc := vm.NewNativeFunction(1, false, "__lookupGetter__", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeUndefined || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		if len(args) < 1 {
			return vm.Undefined, nil
		}
		// Step 2: Let key be ? ToPropertyKey(P).
		keyVal, err := toPropertyKeyValue(vmInstance, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return vm.Undefined, nil
		}
		propName := keyVal.ToString()
		// Walk up the prototype chain looking for an accessor. Only
		// PlainObject/Array carry accessors in this VM's model; other kinds
		// (Function, DictObject, ...) can still hold a matching data property
		// (returning undefined per spec) or sit in the middle of the chain,
		// so they're still visited via vmInstance.PrototypeOf, just without
		// an accessor check.
		current := thisValue
		for depth := 0; depth < 200 && current.Type() != vm.TypeNull && current.Type() != vm.TypeUndefined; depth++ {
			switch current.Type() {
			case vm.TypeObject:
				po := current.AsPlainObject()
				if getter, _, _, _, isAccessor := po.GetOwnAccessor(propName); isAccessor {
					return getter, nil
				}
				if _, hasData := po.GetOwn(propName); hasData {
					return vm.Undefined, nil
				}
			case vm.TypeArray:
				arr := current.AsArray()
				if getter, _, _, _, isAccessor := arr.GetOwnAccessor(propName); isAccessor {
					return getter, nil
				}
				if _, hasData := arr.GetOwn(propName); hasData {
					return vm.Undefined, nil
				}
			default:
				if _, hasData := vmInstance.GetOwnGeneric(current, propName); hasData {
					return vm.Undefined, nil
				}
			}
			current = vmInstance.PrototypeOf(current)
		}
		return vm.Undefined, nil
	})
	objectProto.SetOwnNonEnumerable("__lookupGetter__", lookupGetterFunc)

	// __lookupSetter__ (ES6 B.2.2.5) - legacy method to lookup a setter
	lookupSetterFunc := vm.NewNativeFunction(1, false, "__lookupSetter__", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeUndefined || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		if len(args) < 1 {
			return vm.Undefined, nil
		}
		// Step 2: Let key be ? ToPropertyKey(P).
		keyVal, err := toPropertyKeyValue(vmInstance, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return vm.Undefined, nil
		}
		propName := keyVal.ToString()
		// Walk up the prototype chain looking for an accessor. Only
		// PlainObject/Array carry accessors in this VM's model; other kinds
		// (Function, DictObject, ...) can still hold a matching data property
		// (returning undefined per spec) or sit in the middle of the chain,
		// so they're still visited via vmInstance.PrototypeOf, just without
		// an accessor check.
		current := thisValue
		for depth := 0; depth < 200 && current.Type() != vm.TypeNull && current.Type() != vm.TypeUndefined; depth++ {
			switch current.Type() {
			case vm.TypeObject:
				po := current.AsPlainObject()
				if _, setter, _, _, isAccessor := po.GetOwnAccessor(propName); isAccessor {
					return setter, nil
				}
				if _, hasData := po.GetOwn(propName); hasData {
					return vm.Undefined, nil
				}
			case vm.TypeArray:
				arr := current.AsArray()
				if _, setter, _, _, isAccessor := arr.GetOwnAccessor(propName); isAccessor {
					return setter, nil
				}
				if _, hasData := arr.GetOwn(propName); hasData {
					return vm.Undefined, nil
				}
			default:
				if _, hasData := vmInstance.GetOwnGeneric(current, propName); hasData {
					return vm.Undefined, nil
				}
			}
			current = vmInstance.PrototypeOf(current)
		}
		return vm.Undefined, nil
	})
	objectProto.SetOwnNonEnumerable("__lookupSetter__", lookupSetterFunc)

	// Create Object constructor (length=1 per spec)
	// objectCtorRef is used for the "nor the active function" check in step 1 of 20.1.1.1
	var objectCtorRef vm.Value
	objectCtor := vm.NewNativeFunction(1, true, "Object", func(args []vm.Value) (vm.Value, error) {
		// Per ECMAScript 20.1.1.1 step 1:
		// If NewTarget is neither undefined nor the active function,
		// return OrdinaryCreateFromConstructor(NewTarget, "%Object.prototype%")
		if newTarget := vmInstance.GetNewTarget(); !newTarget.IsUndefined() && !newTarget.StrictlyEquals(objectCtorRef) {
			proto, gpfcErr := vmInstance.GetPrototypeFromConstructor(newTarget, "%ObjectPrototype%")
			if gpfcErr != nil {
				return vm.Undefined, gpfcErr
			}
			return vm.NewObject(proto), nil
		}

		if len(args) == 0 {
			return vm.NewObject(vm.NewValueFromPlainObject(objectProto)), nil
		}
		arg := args[0]

		// If already an object type, return as-is (per spec: ToObject for objects returns the same object)
		if arg.IsObject() || arg.IsCallable() || arg.Type() == vm.TypeArray || arg.Type() == vm.TypeRegExp ||
			arg.Type() == vm.TypeMap || arg.Type() == vm.TypeSet || arg.Type() == vm.TypeProxy ||
			arg.Type() == vm.TypeGenerator || arg.Type() == vm.TypeAsyncGenerator {
			return arg, nil
		}

		// Box primitives into wrapper objects (ECMAScript ToObject operation)
		switch arg.Type() {
		case vm.TypeNull, vm.TypeUndefined:
			// null and undefined throw TypeError in strict mode
			// For now, create an empty object
			return vm.NewObject(vm.NewValueFromPlainObject(objectProto)), nil

		case vm.TypeBigInt:
			// Create BigInt wrapper object
			wrapper := vm.NewObject(vmInstance.BigIntPrototype).AsPlainObject()
			// Store the primitive value internally
			wrapper.SetOwnNonEnumerable("[[PrimitiveValue]]", arg)
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			// Create Number wrapper object
			wrapper := vm.NewObject(vmInstance.NumberPrototype).AsPlainObject()
			wrapper.SetOwnNonEnumerable("[[PrimitiveValue]]", arg)
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeString:
			// Create String wrapper object
			wrapper := vm.NewObject(vmInstance.StringPrototype).AsPlainObject()
			wrapper.SetOwnNonEnumerable("[[PrimitiveValue]]", arg)
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeBoolean:
			// Create Boolean wrapper object
			wrapper := vm.NewObject(vmInstance.BooleanPrototype).AsPlainObject()
			wrapper.SetOwnNonEnumerable("[[PrimitiveValue]]", arg)
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeSymbol:
			// Create Symbol wrapper object
			wrapper := vm.NewObject(vmInstance.SymbolPrototype).AsPlainObject()
			wrapper.SetOwnNonEnumerable("[[PrimitiveValue]]", arg)
			return vm.NewValueFromPlainObject(wrapper), nil

		default:
			// Fallback: create empty object
			return vm.NewObject(vm.NewValueFromPlainObject(objectProto)), nil
		}
	})

	// Make it a proper constructor with static methods
	if objectCtor.Type() == vm.TypeNativeFunction {
		ctorObj := objectCtor.AsNativeFunction()
		// Convert to object with properties
		ctorWithProps := vm.NewConstructorWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(objectProto))
		if v, ok := ctorPropsObj.Properties.GetOwn("prototype"); ok {
			w, e, c := false, false, false
			ctorPropsObj.Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
		}

		// Add static methods
		ctorPropsObj.Properties.SetOwnNonEnumerable("create", vm.NewNativeFunction(2, false, "create", func(args []vm.Value) (vm.Value, error) {
			return objectCreateWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("keys", vm.NewNativeFunction(1, false, "keys", func(args []vm.Value) (vm.Value, error) {
			return objectKeysWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("values", vm.NewNativeFunction(1, false, "values", func(args []vm.Value) (vm.Value, error) {
			return objectValuesWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("entries", vm.NewNativeFunction(1, false, "entries", func(args []vm.Value) (vm.Value, error) {
			return objectEntriesWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("getOwnPropertyNames", vm.NewNativeFunction(1, false, "getOwnPropertyNames", func(args []vm.Value) (vm.Value, error) {
			return objectGetOwnPropertyNamesWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("getOwnPropertySymbols", vm.NewNativeFunction(1, false, "getOwnPropertySymbols", func(args []vm.Value) (vm.Value, error) {
			return objectGetOwnPropertySymbolsWithVM(vmInstance, args)
		}))
		// Reflect-like ownKeys: strings first, then symbols
		ctorPropsObj.Properties.SetOwnNonEnumerable("__ownKeys", vm.NewNativeFunction(1, false, "__ownKeys", reflectOwnKeysImpl))
		ctorPropsObj.Properties.SetOwnNonEnumerable("assign", vm.NewNativeFunction(1, true, "assign", func(args []vm.Value) (vm.Value, error) {
			return objectAssignWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("hasOwn", vm.NewNativeFunction(2, false, "hasOwn", func(args []vm.Value) (vm.Value, error) {
			return objectHasOwnWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("fromEntries", vm.NewNativeFunction(1, false, "fromEntries", objectFromEntriesImpl))
		ctorPropsObj.Properties.SetOwnNonEnumerable("getPrototypeOf", vm.NewNativeFunction(1, false, "getPrototypeOf", func(args []vm.Value) (vm.Value, error) {
			return objectGetPrototypeOfWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("setPrototypeOf", vm.NewNativeFunction(2, false, "setPrototypeOf", func(args []vm.Value) (vm.Value, error) {
			return objectSetPrototypeOfWithVM(vmInstance, args)
		}))
		// defineProperty delegates to the full implementation with symbol/accessor support
		ctorPropsObj.Properties.SetOwnNonEnumerable("defineProperty", vm.NewNativeFunction(3, false, "defineProperty", func(args []vm.Value) (vm.Value, error) {
			return objectDefinePropertyWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("defineProperties", vm.NewNativeFunction(2, false, "defineProperties", func(args []vm.Value) (vm.Value, error) {
			return objectDefinePropertiesWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("getOwnPropertyDescriptor", vm.NewNativeFunction(2, false, "getOwnPropertyDescriptor", func(args []vm.Value) (vm.Value, error) {
			return objectGetOwnPropertyDescriptorWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("getOwnPropertyDescriptors", vm.NewNativeFunction(1, false, "getOwnPropertyDescriptors", func(args []vm.Value) (vm.Value, error) {
			return objectGetOwnPropertyDescriptorsWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("isExtensible", vm.NewNativeFunction(1, false, "isExtensible", func(args []vm.Value) (vm.Value, error) {
			return objectIsExtensibleWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("preventExtensions", vm.NewNativeFunction(1, false, "preventExtensions", func(args []vm.Value) (vm.Value, error) {
			return objectPreventExtensionsWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("freeze", vm.NewNativeFunction(1, false, "freeze", func(args []vm.Value) (vm.Value, error) {
			return objectFreezeWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("seal", vm.NewNativeFunction(1, false, "seal", func(args []vm.Value) (vm.Value, error) {
			return objectSealWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("isFrozen", vm.NewNativeFunction(1, false, "isFrozen", func(args []vm.Value) (vm.Value, error) {
			return objectIsFrozenWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("isSealed", vm.NewNativeFunction(1, false, "isSealed", func(args []vm.Value) (vm.Value, error) {
			return objectIsSealedWithVM(vmInstance, args)
		}))
		// Object.is
		ctorPropsObj.Properties.SetOwnNonEnumerable("is", vm.NewNativeFunction(2, false, "is", func(args []vm.Value) (vm.Value, error) {
			// Missing arguments are treated as undefined per ECMAScript spec
			var x, y vm.Value = vm.Undefined, vm.Undefined
			if len(args) >= 1 {
				x = args[0]
			}
			if len(args) >= 2 {
				y = args[1]
			}
			return vm.BooleanValue(sameValue(x, y)), nil
		}))

		// Object.groupBy(items, callbackfn)
		ctorPropsObj.Properties.SetOwnNonEnumerable("groupBy", vm.NewNativeFunction(2, false, "groupBy", func(args []vm.Value) (vm.Value, error) {
			if len(args) < 2 {
				return vm.Undefined, vmInstance.NewTypeError("Object.groupBy requires 2 arguments")
			}

			items := args[0]
			callbackfn := args[1]

			// Check that callbackfn is callable
			if !callbackfn.IsCallable() {
				return vm.Undefined, vmInstance.NewTypeError("Object.groupBy: callback is not a function")
			}

			// Create result object with null prototype
			result := vm.NewObject(vm.Null).AsPlainObject()

			// Get iterator from items
			var iterator vm.Value
			var iterMethod vm.Value
			var hasIterator bool

			// Handle string type specially - get iterator from String.prototype
			if items.Type() == vm.TypeString {
				if vmInstance.StringPrototype.Type() != vm.TypeUndefined {
					proto := vmInstance.StringPrototype.AsPlainObject()
					if proto != nil {
						iterMethod, hasIterator = proto.GetOwnByKey(vm.NewSymbolKey(SymbolIterator))
					}
				}
			} else {
				iterMethod, hasIterator = vmInstance.GetSymbolProperty(items, SymbolIterator)
			}

			if hasIterator && iterMethod.IsCallable() {
				iter, err := vmInstance.Call(iterMethod, items, []vm.Value{})
				if err != nil {
					return vm.Undefined, err
				}
				iterator = iter
			} else {
				return vm.Undefined, vmInstance.NewTypeError("Object.groupBy: items is not iterable")
			}

			// Iterate over items
			k := 0
			for {
				nextMethod, _ := vmInstance.GetProperty(iterator, "next")
				iterResult, err := vmInstance.Call(nextMethod, iterator, []vm.Value{})
				if err != nil {
					return vm.Undefined, err
				}

				doneVal, _ := vmInstance.GetProperty(iterResult, "done")
				if doneVal.IsTruthy() {
					break
				}

				value, _ := vmInstance.GetProperty(iterResult, "value")

				// Call callback with (value, k)
				keyResult, err := vmInstance.CallArgs2(callbackfn, vm.Undefined, value, vm.NumberValue(float64(k)))
				if err != nil {
					return vm.Undefined, err
				}

				// Coerce key to property key (ToPropertyKey)
				// For objects, call ToPrimitive with "string" hint to get proper toString() call
				var key string
				if keyResult.IsObject() || keyResult.IsCallable() {
					vmInstance.EnterHelperCall()
					primitiveVal := vmInstance.ToPrimitive(keyResult, "string")
					vmInstance.ExitHelperCall()
					// Check if ToPrimitive threw an exception
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.Undefined, nil // Let exception propagate
					}
					key = primitiveVal.ToString()
				} else {
					key = keyResult.ToString()
				}

				// Get or create group array
				var group vm.Value
				if existing, ok := result.GetOwn(key); ok {
					group = existing
				} else {
					group = vm.NewArray()
					result.SetOwn(key, group)
				}

				// Append value to group
				group.AsArray().Append(value)

				k++
			}

			return vm.NewValueFromPlainObject(result), nil
		}))

		objectCtor = ctorWithProps
	}

	// Set objectCtorRef so the constructor's closure can check "nor the active function"
	objectCtorRef = objectCtor

	// Set constructor property on prototype
	objectProto.SetOwnNonEnumerable("constructor", objectCtor)
	if v, ok := objectProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Store in VM
	vmInstance.ObjectPrototype = vm.NewValueFromPlainObject(objectProto)

	// Also store in context so other initializers can use it
	ctx.ObjectPrototype = vmInstance.ObjectPrototype

	// Define globally
	return ctx.DefineGlobal("Object", objectCtor)
}

// toPropertyKeyValue implements ECMAScript ToPropertyKey (§7.1.14).
// It converts a value to a property key (string or symbol), calling ToPrimitive if needed.
// Returns the key as a vm.Value (either TypeString or TypeSymbol).
func toPropertyKeyValue(vmInstance *vm.VM, val vm.Value) (vm.Value, error) {
	// If already a symbol, return directly
	if val.Type() == vm.TypeSymbol {
		return val, nil
	}
	// If already a string/number/boolean primitive, convert to string
	if !val.IsObject() && !val.IsCallable() {
		return vm.NewString(val.ToString()), nil
	}
	// For objects, call ToPrimitive with "string" hint
	vmInstance.EnterHelperCall()
	result := vmInstance.ToPrimitive(val, "string")
	vmInstance.ExitHelperCall()
	// Check if ToPrimitive threw an exception
	if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
		return vm.Undefined, nil // Exception propagates through VM
	}
	// If result is a symbol, return it
	if result.Type() == vm.TypeSymbol {
		return result, nil
	}
	return vm.NewString(result.ToString()), nil
}

// isArrayValueOrError implements the ECMAScript IsArray abstract operation (§7.2.2).
// For revoked Proxy objects, it throws TypeError. For non-revoked proxies, recursively checks target.
func isArrayValueOrError(vmInstance *vm.VM, val vm.Value) (bool, error) {
	for i := 0; i < 100; i++ { // safety limit for proxy chains
		switch val.Type() {
		case vm.TypeArray:
			return true, nil
		case vm.TypeProxy:
			proxy := val.AsProxy()
			if proxy.Revoked {
				return false, vmInstance.NewTypeError("Cannot perform 'IsArray' on a proxy that has been revoked")
			}
			val = proxy.Target()
			continue
		default:
			return false, nil
		}
	}
	return false, nil
}

// lookupToStringTag implements Get(O, @@toStringTag) for toString.
// It properly handles all value types, accessor properties (getters), and prototype chains.
// Returns (tag value, error). Error is non-nil if a getter throws.
func lookupToStringTag(vmInstance *vm.VM, val vm.Value) (vm.Value, error) {
	symKey := vm.NewSymbolKey(vmInstance.SymbolToStringTag)
	symObj := vmInstance.SymbolToStringTag.AsSymbolObject()
	return lookupSymbolProp(vmInstance, val, symKey, symObj, 0)
}

// lookupSymbolProp walks the value's own properties and prototype chain
// looking for a symbol-keyed property. Handles accessor properties by invoking getters.
func lookupSymbolProp(vmInstance *vm.VM, val vm.Value, symKey vm.PropertyKey, symObj *vm.SymbolObject, depth int) (vm.Value, error) {
	if depth > 50 {
		return vm.Undefined, nil // safety limit
	}

	switch val.Type() {
	case vm.TypeProxy:
		proxy := val.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot perform 'get' on a proxy that has been revoked")
		}
		handler := proxy.Handler()
		// GetMethod(handler, "get") per spec: an inherited trap counts, not
		// just an own one - vmInstance.ProxyGetTrap (not a bare
		// handler.AsPlainObject().GetOwn("get")) for the same reason
		// documented on its pkg/vm definition.
		getTrap, hasGetTrap := vmInstance.ProxyGetTrap(handler, "get")
		if hasGetTrap && getTrap.IsCallable() {
			return vmInstance.CallArgs3(getTrap, handler, proxy.Target(), vmInstance.SymbolToStringTag, val)
		}
		// No get trap — check target
		return lookupSymbolProp(vmInstance, proxy.Target(), symKey, symObj, depth+1)

	case vm.TypeObject:
		return lookupSymbolPropInPlainObj(vmInstance, val, val.AsPlainObject(), symKey, symObj, depth)

	case vm.TypeArray:
		arr := val.AsArray()
		if v, ok := arr.GetSymbolProp(symObj); ok {
			return v, nil
		}
		return lookupSymbolPropFromProto(vmInstance, vmInstance.ArrayPrototype, symKey, symObj, depth)

	case vm.TypeFunction:
		fn := val.AsFunction()
		if fn != nil && fn.Properties != nil {
			if v, err := checkPlainObjOwnSymbol(vmInstance, val, fn.Properties, symKey); v.Type() != 0 || err != nil {
				return v, err
			}
			// Walk up the function's __proto__ chain
			proto := fn.Properties.GetPrototype()
			if proto.Type() != vm.TypeNull && !proto.IsUndefined() && proto.Type() != 0 {
				return lookupSymbolPropFromProto(vmInstance, proto, symKey, symObj, depth)
			}
		}
		// Use correct prototype based on function kind
		fnProto, _ := getPrototypeOfValue(vmInstance, val)
		return lookupSymbolPropFromProto(vmInstance, fnProto, symKey, symObj, depth)

	case vm.TypeClosure:
		cl := val.AsClosure()
		if cl != nil {
			// Check closure's own Properties
			if cl.Properties != nil {
				if v, err := checkPlainObjOwnSymbol(vmInstance, val, cl.Properties, symKey); v.Type() != 0 || err != nil {
					return v, err
				}
			}
			// Check underlying function's Properties
			if cl.Fn != nil && cl.Fn.Properties != nil {
				if v, err := checkPlainObjOwnSymbol(vmInstance, val, cl.Fn.Properties, symKey); v.Type() != 0 || err != nil {
					return v, err
				}
				// Walk up the function's __proto__ chain
				proto := cl.Fn.Properties.GetPrototype()
				if proto.Type() != vm.TypeNull && !proto.IsUndefined() && proto.Type() != 0 {
					return lookupSymbolPropFromProto(vmInstance, proto, symKey, symObj, depth)
				}
			}
		}
		// Use correct prototype based on closure kind (generator, async, etc.)
		clProto, _ := getPrototypeOfValue(vmInstance, val)
		return lookupSymbolPropFromProto(vmInstance, clProto, symKey, symObj, depth)

	case vm.TypeNativeFunctionWithProps:
		nfp := val.AsNativeFunctionWithProps()
		if v, err := checkPlainObjOwnSymbol(vmInstance, val, nfp.Properties, symKey); v.Type() != 0 || err != nil {
			return v, err
		}
		proto := nfp.Properties.GetPrototype()
		if proto.Type() != vm.TypeNull && !proto.IsUndefined() && proto.Type() != 0 {
			return lookupSymbolPropFromProto(vmInstance, proto, symKey, symObj, depth)
		}
		return lookupSymbolPropFromProto(vmInstance, vmInstance.FunctionPrototype, symKey, symObj, depth)

	case vm.TypeNativeFunction:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.FunctionPrototype, symKey, symObj, depth)

	case vm.TypeBoundFunction:
		bf := val.AsBoundFunction()
		if bf.Properties != nil {
			if v, err := checkPlainObjOwnSymbol(vmInstance, val, bf.Properties, symKey); v.Type() != 0 || err != nil {
				return v, err
			}
		}
		return lookupSymbolPropFromProto(vmInstance, vmInstance.FunctionPrototype, symKey, symObj, depth)

	case vm.TypeRegExp:
		regex := val.AsRegExpObject()
		if regex != nil && regex.Properties != nil {
			if v, err := checkPlainObjOwnSymbol(vmInstance, val, regex.Properties, symKey); v.Type() != 0 || err != nil {
				return v, err
			}
		}
		return lookupSymbolPropFromProto(vmInstance, vmInstance.RegExpPrototype, symKey, symObj, depth)

	case vm.TypeMap:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.MapPrototype, symKey, symObj, depth)
	case vm.TypeSet:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.SetPrototype, symKey, symObj, depth)
	case vm.TypePromise:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.PromisePrototype, symKey, symObj, depth)
	case vm.TypeGenerator:
		// Check the generator's own prototype chain (genFn.prototype → Generator.prototype)
		genObj := val.AsGenerator()
		if genObj.Prototype != nil {
			return lookupSymbolPropInPlainObj(vmInstance, val, genObj.Prototype, symKey, symObj, depth)
		}
		return lookupSymbolPropFromProto(vmInstance, vmInstance.GeneratorPrototype, symKey, symObj, depth)
	case vm.TypeAsyncGenerator:
		// Check the async generator's own prototype chain
		asyncGenObj := val.AsAsyncGenerator()
		if asyncGenObj.Prototype != nil {
			return lookupSymbolPropInPlainObj(vmInstance, val, asyncGenObj.Prototype, symKey, symObj, depth)
		}
		return lookupSymbolPropFromProto(vmInstance, vmInstance.AsyncGeneratorPrototype, symKey, symObj, depth)
	case vm.TypeArrayBuffer:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.ArrayBufferPrototype, symKey, symObj, depth)
	case vm.TypeSharedArrayBuffer:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.SharedArrayBufferPrototype, symKey, symObj, depth)
	case vm.TypeTypedArray:
		// Resolve the concrete per-kind prototype (or a per-instance override,
		// e.g. from subclassing) rather than ObjectPrototype: well-known symbol
		// properties like @@toStringTag live on the specific XxxArray.prototype
		// (via %TypedArray%.prototype) as an ACCESSOR (its getter reads
		// ta.GetElementType(), varying per concrete subtype). Call
		// lookupSymbolPropInPlainObj directly (rather than going through
		// lookupSymbolPropFromProto -> lookupSymbolProp, which would re-dispatch
		// on the *prototype's* type and lose the original TypedArray as
		// receiver) so the getter is invoked with the TypedArray instance as
		// `this`, not the prototype object.
		proto := typedArrayEffectivePrototype(vmInstance, val)
		if proto.Type() != vm.TypeObject {
			return vm.Undefined, nil
		}
		return lookupSymbolPropInPlainObj(vmInstance, val, proto.AsPlainObject(), symKey, symObj, depth)
	case vm.TypeArguments:
		// Check own symbol properties first
		argObj := val.AsArguments()
		if symObj != nil {
			if v, ok := argObj.GetSymbolProp(symObj); ok {
				return v, nil
			}
		}
		return lookupSymbolPropFromProto(vmInstance, vmInstance.ObjectPrototype, symKey, symObj, depth)

	// Primitives - check their wrapper prototype
	case vm.TypeBoolean:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.BooleanPrototype, symKey, symObj, depth)
	case vm.TypeFloatNumber, vm.TypeIntegerNumber:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.NumberPrototype, symKey, symObj, depth)
	case vm.TypeString:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.StringPrototype, symKey, symObj, depth)
	case vm.TypeSymbol:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.SymbolPrototype, symKey, symObj, depth)
	case vm.TypeBigInt:
		return lookupSymbolPropFromProto(vmInstance, vmInstance.BigIntPrototype, symKey, symObj, depth)

	default:
		return vm.Undefined, nil
	}
}

// lookupSymbolPropFromProto starts a symbol property lookup from a prototype value.
func lookupSymbolPropFromProto(vmInstance *vm.VM, proto vm.Value, symKey vm.PropertyKey, symObj *vm.SymbolObject, depth int) (vm.Value, error) {
	if proto.Type() == vm.TypeNull || proto.IsUndefined() || proto.Type() == 0 {
		return vm.Undefined, nil
	}
	return lookupSymbolProp(vmInstance, proto, symKey, symObj, depth+1)
}

// lookupSymbolPropInPlainObj walks a PlainObject and its prototype chain for a symbol property.
// Handles accessor properties by invoking getters on the original receiver.
func lookupSymbolPropInPlainObj(vmInstance *vm.VM, receiver vm.Value, obj *vm.PlainObject, symKey vm.PropertyKey, symObj *vm.SymbolObject, depth int) (vm.Value, error) {
	for obj != nil && depth < 50 {
		depth++
		// Check for accessor property first
		if getter, _, _, _, isAccessor := obj.GetOwnAccessorByKey(symKey); isAccessor {
			if getter.IsCallable() {
				return vmInstance.Call(getter, receiver, []vm.Value{})
			}
			return vm.Undefined, nil
		}
		// Check for data property
		if tag, ok := obj.GetOwnByKey(symKey); ok {
			return tag, nil
		}
		// Walk up prototype chain
		proto := obj.GetPrototype()
		if proto.Type() == vm.TypeObject {
			obj = proto.AsPlainObject()
		} else if proto.Type() == vm.TypeNativeFunctionWithProps {
			nfp := proto.AsNativeFunctionWithProps()
			// Check accessor on NativeFunctionWithProps
			if getter, _, _, _, isAccessor := nfp.Properties.GetOwnAccessorByKey(symKey); isAccessor {
				if getter.IsCallable() {
					return vmInstance.Call(getter, receiver, []vm.Value{})
				}
				return vm.Undefined, nil
			}
			if tag, ok := nfp.Properties.GetOwnByKey(symKey); ok {
				return tag, nil
			}
			// Continue up nfp's prototype chain
			proto = nfp.Properties.GetPrototype()
			if proto.Type() == vm.TypeObject {
				obj = proto.AsPlainObject()
			} else {
				break
			}
		} else {
			break
		}
	}
	return vm.Undefined, nil
}

// checkPlainObjOwnSymbol checks a PlainObject for an own symbol property (data or accessor).
// Returns (Undefined with type 0, nil) if not found, so caller can continue lookup.
func checkPlainObjOwnSymbol(vmInstance *vm.VM, receiver vm.Value, obj *vm.PlainObject, symKey vm.PropertyKey) (vm.Value, error) {
	if getter, _, _, _, isAccessor := obj.GetOwnAccessorByKey(symKey); isAccessor {
		if getter.IsCallable() {
			result, err := vmInstance.Call(getter, receiver, []vm.Value{})
			return result, err
		}
		return vm.Undefined, nil
	}
	if tag, ok := obj.GetOwnByKey(symKey); ok {
		return tag, nil
	}
	return vm.Value{}, nil // zero-value type (0) signals "not found"
}

// Static method implementations

func objectCreateWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, vmInstance.NewTypeError("Object prototype may only be an Object or null: undefined")
	}

	proto := args[0]

	// undefined and other non-object, non-null values throw TypeError
	if proto.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Object prototype may only be an Object or null: undefined")
	}
	// In JavaScript, any object-like value can be a prototype (functions, arrays, generators, etc.)
	protoIsValid := proto.Type() == vm.TypeNull ||
		proto.IsObject() ||
		proto.IsCallable() ||
		proto.Type() == vm.TypeGenerator ||
		proto.Type() == vm.TypeAsyncGenerator
	if !protoIsValid {
		return vm.Undefined, vmInstance.NewTypeError("Object prototype may only be an Object or null")
	}

	// Create a new object with the specified prototype
	var obj vm.Value
	if proto.Type() == vm.TypeNull {
		// For null prototype, create object and set prototype to null
		obj = vm.NewObject(vm.Null)
		if obj.Type() == vm.TypeObject {
			plainObj := obj.AsPlainObject()
			plainObj.SetPrototype(vm.Null)
		}
	} else {
		// For object prototype, NewObject handles it correctly
		obj = vm.NewObject(proto)
	}

	// If properties descriptor is provided, define properties using defineProperties logic
	if len(args) >= 2 && !args[1].IsUndefined() {
		// Call Object.defineProperties logic
		_, err := objectDefinePropertiesImpl(vmInstance, obj, args[1])
		if err != nil {
			return vm.Undefined, err
		}
	}

	return obj, nil
}

// objectDefinePropertiesImpl is the core implementation for Object.defineProperties and Object.create
// It properly handles getters on descriptor objects per ECMAScript 8.10.5 ToPropertyDescriptor
func objectDefinePropertiesImpl(vmInstance *vm.VM, obj vm.Value, propertiesDesc vm.Value) (vm.Value, error) {
	// Per spec: call ToObject(Properties) first - wraps primitives to their wrapper objects
	if !propertiesDesc.IsObject() && !propertiesDesc.IsCallable() {
		// For strings, ToObject creates a String wrapper with indexed chars as enumerable properties.
		// Since our String wrapper doesn't expose indexed chars, handle it directly:
		// non-empty strings will fail in ToPropertyDescriptor when char values aren't objects.
		if propertiesDesc.Type() == vm.TypeString && len(propertiesDesc.ToString()) > 0 {
			ch := string(propertiesDesc.ToString()[0])
			return vm.Undefined, vmInstance.NewTypeError("Property description must be an object: " + ch)
		}
		var err error
		propertiesDesc, err = vmInstance.ToObject(propertiesDesc)
		if err != nil {
			return vm.Undefined, err
		}
	}

	// Get keys from the properties descriptor
	var keys []string
	switch propertiesDesc.Type() {
	case vm.TypeObject:
		if propertiesDesc.Type() == vm.TypeObject {
			po := propertiesDesc.AsPlainObject()
			for _, key := range po.OwnKeys() {
				if _, _, enumerable, _, ok := po.GetOwnDescriptor(key); ok && enumerable {
					keys = append(keys, key)
				}
			}
		}
	case vm.TypeArray:
		if propertiesDesc.Type() == vm.TypeArray {
			arr := propertiesDesc.AsArray()
			// Include numeric indices
			for i := 0; i < arr.Length(); i++ {
				keys = append(keys, strconv.Itoa(i))
			}
			// Include named properties (like "prop" in the test)
			for _, key := range arr.NamedPropertyKeys() {
				// Check if enumerable
				if _, enumerable, ok := arr.GetNamedPropertyDescriptor(key); ok && enumerable {
					keys = append(keys, key)
				}
			}
		}
	default:
		// For function types and others, try to get as PlainObject if possible
		if propertiesDesc.Type() == vm.TypeObject {
			po := propertiesDesc.AsPlainObject()
			for _, key := range po.OwnKeys() {
				if _, _, enumerable, _, ok := po.GetOwnDescriptor(key); ok && enumerable {
					keys = append(keys, key)
				}
			}
		}
	}

	// Process each property by delegating to objectDefinePropertyWithVM
	// This reuses all validation (extensibility checks, non-configurable violations, etc.)
	for _, key := range keys {
		// Get the property descriptor object
		propDesc, err := vmInstance.GetProperty(propertiesDesc, key)
		if err != nil {
			return vm.Undefined, err
		}

		// Delegate to Object.defineProperty which handles all validation
		_, err = objectDefinePropertyWithVM(vmInstance, []vm.Value{obj, vm.NewString(key), propDesc})
		if err != nil {
			return vm.Undefined, err
		}
	}

	return obj, nil
}

func objectDefinePropertiesWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) < 2 {
		return vm.Undefined, vmInstance.NewTypeError("Object.defineProperties requires at least 2 arguments")
	}

	obj := args[0]
	// Functions are objects in JS (TypeFunction/TypeClosure sit outside the IsObject range).
	if !obj.IsObject() && !obj.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError("Object.defineProperties called on non-object")
	}

	return objectDefinePropertiesImpl(vmInstance, obj, args[1])
}

// getTargetOwnKeys returns own property keys from a target object (non-Proxy path)
func getTargetOwnKeys(vmInstance *vm.VM, target vm.Value) []vm.Value {
	var result []vm.Value
	switch target.Type() {
	case vm.TypeObject:
		po := target.AsPlainObject()
		// OwnPropertyNames includes ALL keys (enumerable + non-enumerable)
		for _, key := range po.OwnPropertyNames() {
			result = append(result, vm.NewString(key))
		}
	case vm.TypeArray:
		arr := target.AsArray()
		for i := 0; i < arr.Length(); i++ {
			result = append(result, vm.NewString(strconv.Itoa(i)))
		}
		// Include "length" as it's an own property of arrays
		result = append(result, vm.NewString("length"))
	case vm.TypeDictObject:
		do := target.AsDictObject()
		for _, key := range do.OwnPropertyNames() {
			result = append(result, vm.NewString(key))
		}
	}
	return result
}

// isTargetExtensible checks if a target object is extensible
func isTargetExtensible(target vm.Value) bool {
	switch target.Type() {
	case vm.TypeObject:
		return target.AsPlainObject().IsExtensible()
	case vm.TypeArray:
		return target.AsArray().IsExtensible()
	case vm.TypeDictObject:
		return target.AsDictObject().IsExtensible()
	}
	return true
}

// isTargetKeyNonConfigurable checks if a key on the target is non-configurable
func isTargetKeyNonConfigurable(target vm.Value, key string) bool {
	switch target.Type() {
	case vm.TypeObject:
		if _, _, _, configurable, ok := target.AsPlainObject().GetOwnDescriptor(key); ok {
			return !configurable
		}
	case vm.TypeArray:
		if key == "length" {
			return true
		}
	}
	return false
}

// isTargetKeyEnumerable checks if a key is enumerable on a target object (non-Proxy path)
func isTargetKeyEnumerable(vmInstance *vm.VM, target vm.Value, key string) bool {
	switch target.Type() {
	case vm.TypeObject:
		po := target.AsPlainObject()
		if _, _, en, _, ok := po.GetOwnDescriptor(key); ok {
			return en
		}
	case vm.TypeArray:
		arr := target.AsArray()
		// Numeric indices are enumerable
		if n, err := strconv.Atoi(key); err == nil && n >= 0 && n < arr.Length() {
			return true
		}
		// "length" is not enumerable
		return false
	case vm.TypeDictObject:
		// DictObject keys are always enumerable
		if target.Type() == vm.TypeDictObject {
			do := target.AsDictObject()
			if _, ok := do.Get(key); ok {
				return true
			}
		}
	}
	return false
}

// arraySparseIndices returns an array's own numeric-index keys that live
// beyond its dense elements bound (a.DenseLength()), as ints in ascending
// numeric order - the order ECMA-262's OrdinaryOwnPropertyKeys requires
// for integer-indexed properties (matching how the dense range below them
// is already visited in order by a plain 0..DenseLength() loop).
//
// A property beyond the dense bound is tracked in one of two disjoint
// places depending on how it was defined (see ArrayDefineOwnProperty,
// array_props.go): a plain data property lives in a.properties (found via
// NamedPropertyKeys), while an accessor - possible at ANY index, not just
// ones past maxDenseArrayDefineIndex; an in-bounds index that never grew
// `elements` far enough to reach it, e.g. index 10000 on a 5-element
// array, is just as "sparse" from this function's point of view - is
// never written to a.properties at all (DefineAccessorProperty only ever
// touches getters/setters/propertyDesc), so it's found via AccessorKeys
// instead. Both must be walked, or an accessor sparse index silently
// disappears from every enumeration below (regressed test262
// built-ins/Object/keys/15.2.3.14-5-14.js during development of this fix,
// which defines exactly such an accessor at index 10000 on a 5-element
// sparse array).
//
// Every array-own-key enumeration below used to loop `0..a.Length()` to
// visit every index, but Length() reports the array's `.length` property -
// which a defineProperty call at a huge index extends without touching
// `elements` at all - so that loop was actually a multi-billion-iteration
// hang for an array otherwise holding a handful of elements, not an O(1)
// per real entry scan (paserati#176/#178). This walks NamedPropertyKeys()
// and AccessorKeys() instead - O(number of sparse/named/accessor entries),
// never O(index value) - keeping only the ones that parse as a valid array
// index (vm.ParseArrayIndex, which already enforces the same 2^32-2 upper
// bound ArrayDefineOwnProperty itself uses, so nothing here can
// accidentally treat an out-of-range numeric-looking key as an index).
//
// When enumerableOnly is true, only keys whose tracked descriptor reports
// Enumerable are included (Object.keys/values/entries and Object.assign's
// own-enumerable-properties rule); pass false for an operation that wants
// every own index key regardless of enumerability (Object.
// getOwnPropertyNames, Reflect.ownKeys).
func arraySparseIndices(a *vm.ArrayObject, enumerableOnly bool) []int {
	dense := a.DenseLength()
	seen := make(map[int]bool)
	var idxs []int
	consider := func(key string) {
		idx, isIndex := vm.ParseArrayIndex(key)
		if !isIndex || idx < dense || seen[idx] {
			return
		}
		if enumerableOnly {
			if _, _, enumerable, _, isAccessor := a.GetOwnAccessor(key); isAccessor {
				if !enumerable {
					return
				}
			} else if _, desc, ok := a.GetOwnPropertyDescriptor(key); !ok || !desc.Enumerable {
				return
			}
		}
		seen[idx] = true
		idxs = append(idxs, idx)
	}
	for _, key := range a.NamedPropertyKeys() {
		consider(key)
	}
	for _, key := range a.AccessorKeys() {
		consider(key)
	}
	sort.Ints(idxs)
	return idxs
}

// arraySparseIndexValue reads the value an own sparse index (one
// arraySparseIndices already found - past DenseLength(), stored beyond
// the elements slice) currently holds: calls its getter if it's an
// accessor property, otherwise reads the plain data value from the
// properties map (GetOwn). Deliberately NOT arrayLikeGet
// (array_generic.go): that helper falls back to arrayIndexGetFromProto
// once arr.HasIndex(i) is false, which only walks the PROTOTYPE chain -
// it never consults the array's OWN properties map, so it would report a
// sparse own data property (as opposed to an accessor, which it does
// check first) as absent/inherited instead of returning its real value.
// That's a real, separate gap in arrayLikeGet - same bug class as
// paserati#176, just in a helper #176's own fix never touched - flagged
// as its own follow-up rather than fixed here, since every other
// arrayLikeGet caller (forEach/map/filter/...) needs it too and this
// function's scope is the enumeration hang, not that helper.
func arraySparseIndexValue(vmInstance *vm.VM, a *vm.ArrayObject, receiver vm.Value, idx int) (vm.Value, error) {
	key := strconv.Itoa(idx)
	if getter, _, _, _, isAccessor := a.GetOwnAccessor(key); isAccessor {
		if getter.Type() == vm.TypeUndefined {
			return vm.Undefined, nil
		}
		return vmInstance.Call(getter, receiver, nil)
	}
	if v, ok := a.GetOwn(key); ok {
		return v, nil
	}
	return vm.Undefined, nil
}

// arrayDenseIndexValue reads the value at a dense-range array index (i <
// DenseLength()), calling its getter if Object.defineProperty installed
// an accessor there instead of blindly trusting arrObj.Get(i), which reads
// `elements` directly with no accessor check at all. An index accessor is
// never written into elements in the first place - see
// ArrayDefineOwnProperty's own doc comment ("Every read path that knows
// about per-index accessors ... checks GetOwnAccessor before ever
// consulting the elements slice, so nothing needs to be written there") -
// this is exactly one of those read paths, previously missed:
//
//	const arr = [1, 2, 3];
//	Object.defineProperty(arr, "1", { get() { return 99; }, enumerable: true });
//	arr[1];                    // 99 - the index-access path already checks
//	Object.assign({}, arr)[1]; // before: 2 (stale element) - Node: 99
//
// The sparse half of this same read (arraySparseIndexValue, above) already
// gets it right; this is its dense-range counterpart.
func arrayDenseIndexValue(vmInstance *vm.VM, a *vm.ArrayObject, receiver vm.Value, i int) (vm.Value, error) {
	key := strconv.Itoa(i)
	if getter, _, _, _, isAccessor := a.GetOwnAccessor(key); isAccessor {
		if getter.Type() == vm.TypeUndefined {
			return vm.Undefined, nil
		}
		return vmInstance.Call(getter, receiver, nil)
	}
	return a.Get(i), nil
}

// arrayNamedKeys returns an array's own named (non-index) property keys:
// AccessorKeys() (an accessor installed via Object.defineProperty lives in
// getters/setters, never in `properties` - see ArrayObject.
// DefineAccessorProperty's own doc comment, which explicitly deletes the
// properties entry when converting a plain property to an accessor, so a
// given name lives in at most one of the two stores at a time) followed by
// NamedPropertyKeys() (plain data). Both are filtered with
// vm.ParseArrayIndex - not vm.LooksLikeArrayIndex, which has no upper
// bound and would let an out-of-range numeric key like "4294967295" slip
// past both this filter and arraySparseIndices' own vm.ParseArrayIndex
// filter, vanishing from enumeration entirely - to exclude a sparse index
// sharing either map, which arraySparseIndices already covers.
//
// Before this, a named ACCESSOR property was invisible to every array
// enumeration/collection operation that walked NamedPropertyKeys() alone:
//
//	const arr = [1, 2];
//	Object.defineProperty(arr, "foo", { get() { return 5; }, enumerable: true });
//	Object.keys(arr); // before: ["0","1"] - Node: ["0","1","foo"]
//
// enumerableOnly mirrors arraySparseIndices' own parameter: true for
// for-in/Object.keys/values/entries/Object.assign (own-ENUMERABLE-
// properties only, per CopyDataProperties/EnumerableOwnPropertyNames),
// false for Object.getOwnPropertyNames/Reflect.ownKeys (every own key
// regardless of enumerability).
func arrayNamedKeys(a *vm.ArrayObject, enumerableOnly bool) []string {
	var keys []string
	for _, name := range a.AccessorKeys() {
		if _, isIndex := vm.ParseArrayIndex(name); isIndex {
			continue
		}
		if enumerableOnly {
			if _, _, enumerable, _, isAccessor := a.GetOwnAccessor(name); !isAccessor || !enumerable {
				continue
			}
		}
		keys = append(keys, name)
	}
	for _, name := range a.NamedPropertyKeys() {
		if _, isIndex := vm.ParseArrayIndex(name); isIndex {
			continue
		}
		if enumerableOnly {
			if _, enumerable, ok := a.GetNamedPropertyDescriptor(name); !ok || !enumerable {
				continue
			}
		}
		keys = append(keys, name)
	}
	return keys
}

// arrayNamedKeyValue reads the value of a named (non-index) own property
// arrayNamedKeys already found, calling its getter if it's an accessor -
// the named-key counterpart to arrayDenseIndexValue/arraySparseIndexValue,
// for Object.values/entries.
func arrayNamedKeyValue(vmInstance *vm.VM, a *vm.ArrayObject, receiver vm.Value, name string) (vm.Value, error) {
	if getter, _, _, _, isAccessor := a.GetOwnAccessor(name); isAccessor {
		if getter.Type() == vm.TypeUndefined {
			return vm.Undefined, nil
		}
		return vmInstance.Call(getter, receiver, nil)
	}
	if v, _, ok := a.GetNamedPropertyDescriptor(name); ok {
		return v, nil
	}
	return vm.Undefined, nil
}

func objectKeysWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined to object")
	}

	obj := args[0]
	// ECMAScript: throw TypeError for null/undefined
	if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}
	// ToObject for primitives (strings have enumerable indexed properties)
	if !obj.IsObject() && !obj.IsCallable() {
		converted, err := vmInstance.ToObject(obj)
		if err != nil {
			return vm.NewArray(), nil
		}
		obj = converted
	}

	keys := vm.NewArray()
	keysArray := keys.AsArray()

	// Handle Proxy objects per ECMAScript spec
	if obj.Type() == vm.TypeProxy {
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot get keys of revoked Proxy")
		}

		handler := proxy.Handler()
		target := proxy.Target()

		// === [[OwnPropertyKeys]] proxy internal method ===
		// Step 1: GetMethod(handler, "ownKeys") - uses [[Get]] which is observable
		ownKeysTrap, err := vmInstance.GetProperty(handler, "ownKeys")
		if err != nil {
			return vm.Undefined, err
		}

		var ownKeysList []vm.Value
		// GetMethod semantics: undefined/null → no trap; non-callable → TypeError
		if !ownKeysTrap.IsUndefined() && ownKeysTrap.Type() != vm.TypeNull && !ownKeysTrap.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("'ownKeys' on proxy: trap is not a function")
		}
		if !ownKeysTrap.IsUndefined() && ownKeysTrap.Type() != vm.TypeNull && ownKeysTrap.IsCallable() {
			// Call handler.ownKeys(target)
			result, cerr := vmInstance.Call(ownKeysTrap, handler, []vm.Value{target})
			if cerr != nil {
				return vm.Undefined, cerr
			}
			// CreateListFromArrayLike(trapResultArray, « String, Symbol ») - step 8
			if !result.IsObject() {
				return vm.Undefined, vmInstance.NewTypeError("CreateListFromArrayLike called on non-object")
			}
			if result.Type() == vm.TypeArray {
				arr := result.AsArray()
				for i := 0; i < arr.Length(); i++ {
					elem := arr.Get(i)
					if elem.Type() != vm.TypeString && !elem.IsSymbol() {
						return vm.Undefined, vmInstance.NewTypeError("'ownKeys' on proxy: trap result included a non-string, non-symbol key")
					}
					ownKeysList = append(ownKeysList, elem)
				}
			} else {
				lenVal, lerr := vmInstance.GetProperty(result, "length")
				if lerr != nil {
					return vm.Undefined, lerr
				}
				length := int(lenVal.ToFloat())
				for i := 0; i < length; i++ {
					val, gerr := vmInstance.GetProperty(result, strconv.Itoa(i))
					if gerr != nil {
						return vm.Undefined, gerr
					}
					if val.Type() != vm.TypeString && !val.IsSymbol() {
						return vm.Undefined, vmInstance.NewTypeError("'ownKeys' on proxy: trap result included a non-string, non-symbol key")
					}
					ownKeysList = append(ownKeysList, val)
				}
			}

			// Step 9: Check for duplicate entries in trapResult
			seen := make(map[string]bool)
			for _, k := range ownKeysList {
				var keyStr string
				if k.Type() == vm.TypeString {
					keyStr = "s:" + vm.AsString(k)
				} else {
					keyStr = "y:" + k.ToString()
				}
				if seen[keyStr] {
					return vm.Undefined, vmInstance.NewTypeError("'ownKeys' on proxy: trap returned duplicate entries")
				}
				seen[keyStr] = true
			}

			// [[OwnPropertyKeys]] invariant validation (spec steps 11-22)
			targetKeys := getTargetOwnKeys(vmInstance, target)
			extensible := isTargetExtensible(target)

			// Build set of trap result string keys for lookup
			trapResultSet := make(map[string]bool)
			for _, k := range ownKeysList {
				if k.Type() == vm.TypeString {
					trapResultSet[vm.AsString(k)] = true
				}
			}

			// Build set of target keys for reverse lookup
			targetKeySet := make(map[string]bool)
			var targetNonconfigurableKeys []string
			var targetConfigurableKeys []string
			for _, tk := range targetKeys {
				tkStr := vm.AsString(tk)
				targetKeySet[tkStr] = true
				if isTargetKeyNonConfigurable(target, tkStr) {
					targetNonconfigurableKeys = append(targetNonconfigurableKeys, tkStr)
				} else {
					targetConfigurableKeys = append(targetConfigurableKeys, tkStr)
				}
			}

			// Step 19: All non-configurable target keys must be in trapResult
			for _, key := range targetNonconfigurableKeys {
				if !trapResultSet[key] {
					return vm.Undefined, vmInstance.NewTypeError("'ownKeys' on proxy: trap result did not include '" + key + "'")
				}
			}

			// Steps 20-22: If target is not extensible, trapResult can't have extra keys
			if !extensible {
				// All target configurable keys must be in trapResult
				for _, key := range targetConfigurableKeys {
					if !trapResultSet[key] {
						return vm.Undefined, vmInstance.NewTypeError("'ownKeys' on proxy: trap result did not include '" + key + "'")
					}
				}
				// No extra keys allowed: all trap result keys must be in target
				for _, k := range ownKeysList {
					if k.Type() == vm.TypeString {
						if !targetKeySet[vm.AsString(k)] {
							return vm.Undefined, vmInstance.NewTypeError("'ownKeys' on proxy: trap returned extra keys for non-extensible target")
						}
					}
				}
			}
		} else {
			// No ownKeys trap: fall through to target.[[OwnPropertyKeys]]()
			targetKeys := getTargetOwnKeys(vmInstance, target)
			ownKeysList = targetKeys
		}

		// === EnumerableOwnPropertyNames - filter by enumerability ===
		// Per spec, each key goes through O.[[GetOwnProperty]](key) which is
		// the proxy [[GetOwnProperty]] internal method - this calls
		// GetMethod(handler, "getOwnPropertyDescriptor") on EACH key (observable)
		for _, key := range ownKeysList {
			if key.Type() != vm.TypeString {
				continue
			}

			// Proxy [[GetOwnProperty]]: GetMethod(handler, "getOwnPropertyDescriptor")
			getOwnPropDescTrap, gerr := vmInstance.GetProperty(handler, "getOwnPropertyDescriptor")
			if gerr != nil {
				return vm.Undefined, gerr
			}

			if !getOwnPropDescTrap.IsUndefined() && getOwnPropDescTrap.IsCallable() {
				descResult, derr := vmInstance.CallArgs2(getOwnPropDescTrap, handler, target, key)
				if derr != nil {
					return vm.Undefined, derr
				}
				if descResult.IsUndefined() {
					continue
				}
				enumVal, eerr := vmInstance.GetProperty(descResult, "enumerable")
				if eerr != nil {
					return vm.Undefined, eerr
				}
				if !enumVal.IsTruthy() {
					continue
				}
			} else {
				// No getOwnPropertyDescriptor trap: check target's own property
				enumerable := isTargetKeyEnumerable(vmInstance, target, vm.AsString(key))
				if !enumerable {
					continue
				}
			}

			keysArray.Append(key)
		}
		return keys, nil
	}

	// Handle regular objects - check type before calling As* methods to avoid panic
	switch obj.Type() {
	case vm.TypeObject:
		plainObj := obj.AsPlainObject()
		for _, key := range plainObj.OwnKeys() {
			if _, _, en, _, ok := plainObj.GetOwnDescriptor(key); ok && en {
				keysArray.Append(vm.NewString(key))
			}
		}
	case vm.TypeDictObject:
		dictObj := obj.AsDictObject()
		for _, key := range dictObj.OwnKeys() {
			keysArray.Append(vm.NewString(key))
		}
	case vm.TypeArray:
		arrObj := obj.AsArray()
		for i := 0; i < arrObj.DenseLength(); i++ {
			key := strconv.Itoa(i)
			if !arrObj.HasOwnIndexProperty(key, i) {
				continue // hole - not an own property at all (paserati#300)
			}
			keysArray.Append(vm.NewString(key))
		}
		// A sparse index beyond the dense range (paserati#176/#178 - see
		// arraySparseIndices) still needs to appear here, in ascending
		// numeric order right after the dense indices - not iterate up to
		// it, which is exactly the multi-billion-iteration hang this fixes.
		for _, idx := range arraySparseIndices(arrObj, true) {
			keysArray.Append(vm.NewString(strconv.Itoa(idx)))
		}
		// Named own properties (an exec result's index/input/groups/indices,
		// or anything stored on the array, accessor or plain) follow the
		// indices - see arrayNamedKeys' own doc comment for why this can't
		// be NamedPropertyKeys() alone.
		for _, key := range arrayNamedKeys(arrObj, true) {
			keysArray.Append(vm.NewString(key))
		}
	case vm.TypeArguments:
		argsObj := obj.AsArguments()
		// Arguments object: return numeric indices as keys
		for i := 0; i < argsObj.Length(); i++ {
			keysArray.Append(vm.NewString(strconv.Itoa(i)))
		}
	case vm.TypeFunction:
		funcObj := obj.AsFunction()
		if funcObj.Properties != nil {
			for _, key := range funcObj.Properties.OwnKeys() {
				if _, _, en, _, ok := funcObj.Properties.GetOwnDescriptor(key); ok && en {
					keysArray.Append(vm.NewString(key))
				}
			}
		}
	case vm.TypeClosure:
		closure := obj.AsClosure()
		if closure.Properties != nil {
			for _, key := range closure.Properties.OwnKeys() {
				if _, _, en, _, ok := closure.Properties.GetOwnDescriptor(key); ok && en {
					keysArray.Append(vm.NewString(key))
				}
			}
		}
	case vm.TypeBoundFunction:
		// A bound function's own properties (whether from direct assignment
		// or Object.assign) live on its side table same as a plain
		// function's - this case was missing entirely, so Object.keys on a
		// bound function always came back empty even after `bound.x = 1`
		// (paserati#254).
		bf := obj.AsBoundFunction()
		if bf.Properties != nil {
			for _, key := range bf.Properties.OwnKeys() {
				if _, _, en, _, ok := bf.Properties.GetOwnDescriptor(key); ok && en {
					keysArray.Append(vm.NewString(key))
				}
			}
		}
	case vm.TypeRegExp, vm.TypeMap, vm.TypeSet, vm.TypePromise, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps:
		// Same side table as the exotic kinds above (OwnPropertiesTable,
		// pkg/vm/properties_table.go) - this case was missing entirely, so
		// Object.keys always came back empty for these six kinds even
		// after a custom own property was defined on one (e.g. r.custom =
		// 42, or Object.defineProperty once that gap is fixed too).
		// RegExp's "lastIndex" never appears here: it's a real Go field on
		// RegExpObject, not a side-table entry, and it's non-enumerable in
		// any case (Object.getOwnPropertyDescriptor's TypeRegExp case,
		// same file). TypeNativeFunction/TypeNativeFunctionWithProps'
		// "name"/"length" intrinsics are non-enumerable synthesized
		// properties, not side-table entries either, so they're correctly
		// excluded here the same way.
		if props := vm.OwnPropertiesTable(obj); props != nil {
			for _, key := range props.OwnKeys() {
				if _, _, en, _, ok := props.GetOwnDescriptor(key); ok && en {
					keysArray.Append(vm.NewString(key))
				}
			}
		}
	}

	return keys, nil
}

// (Removed duplicate alternate implementations of objectValuesImpl and objectEntriesImpl)

func objectGetPrototypeOfWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, nil
	}

	obj := args[0]

	// Per spec: ToObject first — converts primitives to their wrapper types
	// Null and undefined throw TypeError
	if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}

	// Exotic-type instances (subclassing a native constructor, or a custom
	// newTarget.prototype via Reflect.construct/GetPrototypeFromConstructor)
	// carry a per-instance [[Prototype]] override that takes precedence over
	// the type-switch below's intrinsic defaults; see InstancePrototypeOverride.
	if override, ok := vmInstance.InstancePrototypeOverride(obj); ok {
		return override, nil
	}

	// Handle primitive types by returning their prototype directly
	if obj.IsNumber() {
		return vmInstance.NumberPrototype, nil
	}
	if obj.Type() == vm.TypeBoolean {
		return vmInstance.BooleanPrototype, nil
	}
	if obj.Type() == vm.TypeString {
		return vmInstance.StringPrototype, nil
	}
	if obj.Type() == vm.TypeSymbol {
		return vmInstance.SymbolPrototype, nil
	}
	if obj.Type() == vm.TypeBigInt {
		return vmInstance.BigIntPrototype, nil
	}

	// For objects with prototypes, return their prototype
	switch obj.Type() {
	case vm.TypeObject:
		// For plain objects, get their actual prototype
		plainObj := obj.AsPlainObject()
		if plainObj != nil {
			return plainObj.GetPrototype(), nil
		}
		return vm.Null, nil
	case vm.TypeArray:
		// For arrays, return Array.prototype
		return vmInstance.ArrayPrototype, nil
	case vm.TypeArguments:
		// For arguments objects, return Object.prototype
		return vmInstance.ObjectPrototype, nil
	case vm.TypeString:
		// For strings, return String.prototype
		return vmInstance.StringPrototype, nil
	case vm.TypeFunction:
		// For functions, return their [[Prototype]]
		fn := obj.AsFunction()
		if fn != nil && fn.Prototype.Type() != vm.TypeNull && fn.Prototype.Type() != vm.TypeUndefined {
			return fn.Prototype, nil
		}
		return vm.Null, nil
	case vm.TypeClosure:
		// For closures, return their function's [[Prototype]]
		cl := obj.AsClosure()
		if cl != nil && cl.Fn != nil && (cl.Fn.Prototype.Type() != vm.TypeNull && cl.Fn.Prototype.Type() != vm.TypeUndefined) {
			return cl.Fn.Prototype, nil
		}
		return vm.Null, nil
	case vm.TypeNativeFunctionWithProps:
		// Per ECMAScript spec, all built-in functions have Function.prototype as their
		// [[Prototype]], unless explicitly set otherwise (e.g. TypedArray constructors
		// whose [[Prototype]] is %TypedArray%).
		nfp := obj.AsNativeFunctionWithProps()
		if nfp != nil && nfp.Properties != nil {
			proto := nfp.Properties.GetPrototype()
			// Check if a custom prototype was explicitly set (not the default ObjectPrototype)
			if proto.Type() != vm.TypeUndefined && proto.Type() != vm.TypeNull && proto != vm.DefaultObjectPrototype {
				return proto, nil
			}
		}
		return vmInstance.FunctionPrototype, nil
	case vm.TypeSet:
		// For Sets, return Set.prototype
		return vmInstance.SetPrototype, nil
	case vm.TypeMap:
		// For Maps, return Map.prototype
		return vmInstance.MapPrototype, nil
	case vm.TypeWeakMap:
		// For WeakMaps, return per-instance prototype or default WeakMapPrototype
		wm := obj.AsWeakMap()
		if wm != nil && wm.GetPrototype().Type() != vm.TypeUndefined {
			return wm.GetPrototype(), nil
		}
		return vmInstance.WeakMapPrototype, nil
	case vm.TypeGenerator:
		// For generators, return their custom prototype or GeneratorPrototype
		genObj := obj.AsGenerator()
		if genObj != nil && genObj.Prototype != nil {
			return vm.NewValueFromPlainObject(genObj.Prototype), nil
		}
		// Return the default GeneratorPrototype
		return vm.Null, nil // TODO: Return proper GeneratorPrototype
	case vm.TypeAsyncGenerator:
		// For async generators, return their custom prototype or AsyncGeneratorPrototype
		asyncGenObj := obj.AsAsyncGenerator()
		if asyncGenObj != nil && asyncGenObj.Prototype != nil {
			return vm.NewValueFromPlainObject(asyncGenObj.Prototype), nil
		}
		// Return the default AsyncGeneratorPrototype
		return vm.Null, nil // TODO: Return proper AsyncGeneratorPrototype
	case vm.TypeProxy:
		// For proxies, call the getPrototypeOf trap if present
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot get prototype of revoked Proxy")
		}

		// Check if handler has a getPrototypeOf trap. GetMethod(handler,
		// "getPrototypeOf") per spec: an inherited trap counts, not just
		// an own one - vmInstance.ProxyGetTrap (not a bare
		// proxy.Handler().AsPlainObject().GetOwn("getPrototypeOf")) for
		// the same reason documented on its pkg/vm definition.
		if trap, ok := vmInstance.ProxyGetTrap(proxy.Handler(), "getPrototypeOf"); ok && !trap.IsUndefined() && trap.Type() != vm.TypeNull {
			// Validate trap is callable
			if !trap.IsFunction() {
				return vm.Undefined, vmInstance.NewTypeError("'getPrototypeOf' on proxy: trap is not a function")
			}

			// Call handler.getPrototypeOf(target)
			result, err := vmInstance.Call(trap, proxy.Handler(), []vm.Value{proxy.Target()})
			if err != nil {
				return vm.Undefined, err
			}

			// Validate result is object or null
			if result.Type() != vm.TypeObject && result.Type() != vm.TypeNull {
				return vm.Undefined, vmInstance.NewTypeError("'getPrototypeOf' on proxy: trap returned neither object nor null")
			}
			return result, nil
		}

		// No trap, delegate to target
		return objectGetPrototypeOfWithVM(vmInstance, []vm.Value{proxy.Target()})
	case vm.TypePromise:
		// For promises, return Promise.prototype
		return vmInstance.PromisePrototype, nil
	case vm.TypeNativeFunction, vm.TypeBoundFunction, vm.TypeAsyncNativeFunction:
		// For native functions and bound functions, return Function.prototype
		return vmInstance.FunctionPrototype, nil
	case vm.TypeTypedArray:
		// For TypedArrays, return the appropriate TypedArray prototype
		ta := obj.AsTypedArray()
		if ta == nil {
			return vm.Null, nil
		}
		switch ta.GetElementType() {
		case vm.TypedArrayUint8:
			return vmInstance.Uint8ArrayPrototype, nil
		case vm.TypedArrayInt8:
			return vmInstance.Int8ArrayPrototype, nil
		case vm.TypedArrayUint16:
			return vmInstance.Uint16ArrayPrototype, nil
		case vm.TypedArrayInt16:
			return vmInstance.Int16ArrayPrototype, nil
		case vm.TypedArrayUint32:
			return vmInstance.Uint32ArrayPrototype, nil
		case vm.TypedArrayInt32:
			return vmInstance.Int32ArrayPrototype, nil
		case vm.TypedArrayFloat16:
			return vmInstance.Float16ArrayPrototype, nil
		case vm.TypedArrayFloat32:
			return vmInstance.Float32ArrayPrototype, nil
		case vm.TypedArrayFloat64:
			return vmInstance.Float64ArrayPrototype, nil
		case vm.TypedArrayUint8Clamped:
			return vmInstance.Uint8ClampedArrayPrototype, nil
		case vm.TypedArrayBigInt64:
			return vmInstance.BigInt64ArrayPrototype, nil
		case vm.TypedArrayBigUint64:
			return vmInstance.BigUint64ArrayPrototype, nil
		default:
			return vmInstance.TypedArrayPrototype, nil
		}
	case vm.TypeArrayBuffer:
		// For ArrayBuffers, return ArrayBuffer.prototype
		return vmInstance.ArrayBufferPrototype, nil
	case vm.TypeDataView:
		// For DataViews, return DataView.prototype
		return vmInstance.DataViewPrototype, nil
	case vm.TypeSharedArrayBuffer:
		// For SharedArrayBuffers, return SharedArrayBuffer.prototype
		return vmInstance.SharedArrayBufferPrototype, nil
	default:
		// For primitive values, return null
		return vm.Null, nil
	}
}

func objectSetPrototypeOfWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) < 2 {
		return vm.Undefined, vmInstance.NewTypeError("Object.setPrototypeOf requires 2 arguments")
	}

	obj := args[0]
	proto := args[1]

	// Second argument must be an object or null
	// In JavaScript, any object-like value can be a prototype (functions, arrays, generators, etc.)
	// Use IsObject() which checks for PlainObject/DictObject/Array/etc., or callable check for functions
	protoIsValid := proto.Type() == vm.TypeNull ||
		proto.IsObject() ||
		proto.IsCallable() ||
		proto.Type() == vm.TypeGenerator ||
		proto.Type() == vm.TypeAsyncGenerator
	if !protoIsValid {
		return vm.Undefined, vmInstance.NewTypeError("Object prototype may only be an Object or null")
	}

	// Handle Proxy objects
	if obj.Type() == vm.TypeProxy {
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot set prototype of revoked Proxy")
		}

		// Check for setPrototypeOf trap. GetMethod(handler, "setPrototypeOf")
		// per spec: an inherited trap counts, not just an own one -
		// vmInstance.ProxyGetTrap (not a bare
		// proxy.Handler().AsPlainObject().GetOwn("setPrototypeOf")) for
		// the same reason documented on its pkg/vm definition.
		if setProtoTrap, ok := vmInstance.ProxyGetTrap(proxy.Handler(), "setPrototypeOf"); ok && !setProtoTrap.IsUndefined() && setProtoTrap.Type() != vm.TypeNull {
			// Validate trap is callable
			if !setProtoTrap.IsFunction() {
				return vm.Undefined, vmInstance.NewTypeError("'setPrototypeOf' on proxy: trap is not a function")
			}

			// Call handler.setPrototypeOf(target, proto)
			result, err := vmInstance.Call(setProtoTrap, proxy.Handler(), []vm.Value{proxy.Target(), proto})
			if err != nil {
				return vm.Undefined, err
			}

			// Result should be boolean - if false, throw
			if result.IsFalsey() {
				return vm.Undefined, vmInstance.NewTypeError("'setPrototypeOf' on proxy: trap returned falsish")
			}

			return obj, nil
		}

		// No trap, delegate to target
		return objectSetPrototypeOfWithVM(vmInstance, []vm.Value{proxy.Target(), proto})
	}

	// First argument must be an object (including functions, arrays, etc.)
	// In JavaScript, functions are objects and their [[Prototype]] can be changed
	objIsObject := obj.Type() == vm.TypeObject ||
		obj.IsCallable() ||
		obj.Type() == vm.TypeArray ||
		obj.Type() == vm.TypeGenerator ||
		obj.Type() == vm.TypeAsyncGenerator ||
		obj.Type() == vm.TypeRegExp ||
		obj.Type() == vm.TypeMap ||
		obj.Type() == vm.TypeSet
	if !objIsObject {
		return vm.Undefined, vmInstance.NewTypeError("Object.setPrototypeOf called on non-object")
	}

	// Module Namespace Exotic Object [[SetPrototypeOf]] behavior (ECMAScript 10.4.6.3)
	// Uses SetImmutablePrototype which returns true if V is same as [[Prototype]], false otherwise
	// For namespace objects, [[Prototype]] is always null
	if obj.Type() == vm.TypeObject {
		if plainObj := obj.AsPlainObject(); plainObj != nil && plainObj.IsModuleNamespace() {
			// Namespace prototype is always null
			if proto.Type() == vm.TypeNull {
				return obj, nil // Success - proto matches
			}
			return vm.Undefined, vmInstance.NewTypeError("Cannot set prototype of immutable prototype exotic object")
		}
	}

	// Set the prototype based on object type
	success := true
	switch obj.Type() {
	case vm.TypeObject:
		if obj.Type() == vm.TypeObject {
			plainObj := obj.AsPlainObject()
			success = plainObj.SetPrototype(proto)
		} else if obj.Type() == vm.TypeDictObject {
			dictObj := obj.AsDictObject()
			success = dictObj.SetPrototype(proto)
		}
	case vm.TypeFunction:
		// For FunctionObject, set the Prototype field
		fn := obj.AsFunction()
		fn.Prototype = proto
	case vm.TypeClosure:
		// For closures, set the underlying function's Prototype field
		closure := obj.AsClosure()
		closure.Fn.Prototype = proto
	case vm.TypeNativeFunction:
		// Native functions - success but no actual prototype storage
		success = true
	case vm.TypeNativeFunctionWithProps:
		// NativeFunctionWithProps - success but no actual prototype storage
		success = true
	case vm.TypeBoundFunction:
		// Bound functions don't have their own prototype
		success = true
	default:
		// For other object types (Map, Set, Generator, etc.), try setting via AsPlainObject
		if obj.Type() == vm.TypeObject {
			plainObj := obj.AsPlainObject()
			success = plainObj.SetPrototype(proto)
		}
	}

	if !success {
		return vm.Undefined, vmInstance.NewTypeError("Cannot set prototype of non-extensible object")
	}

	// Return the object
	return obj, nil
}

func objectValuesWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined to object")
	}

	obj := args[0]
	// ECMAScript: throw TypeError for null/undefined
	if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}
	// ToObject for primitives
	if !obj.IsObject() && !obj.IsCallable() {
		converted, err := vmInstance.ToObject(obj)
		if err != nil {
			return vm.NewArray(), nil
		}
		obj = converted
	}

	values := vm.NewArray()
	valuesArray := values.AsArray()

	switch obj.Type() {
	case vm.TypeObject:
		plainObj := obj.AsPlainObject()
		for _, key := range plainObj.OwnKeys() {
			if _, _, en, _, ok := plainObj.GetOwnDescriptor(key); ok && en {
				value, _ := plainObj.GetOwn(key)
				valuesArray.Append(value)
			}
		}
	case vm.TypeDictObject:
		dictObj := obj.AsDictObject()
		for _, key := range dictObj.OwnKeys() {
			value, _ := dictObj.GetOwn(key)
			valuesArray.Append(value)
		}
	case vm.TypeArray:
		arrObj := obj.AsArray()
		for i := 0; i < arrObj.DenseLength(); i++ {
			key := strconv.Itoa(i)
			if !arrObj.HasOwnIndexProperty(key, i) {
				continue // hole - not an own property at all (paserati#300)
			}
			// arrayDenseIndexValue, not arrObj.Get(i) - see that function's
			// own doc comment for why a plain element read misses an index
			// accessor installed via Object.defineProperty.
			value, err := arrayDenseIndexValue(vmInstance, arrObj, obj, i)
			if err != nil {
				return vm.Undefined, err
			}
			valuesArray.Append(value)
		}
		// A sparse index beyond the dense range (paserati#176/#178 - see
		// arraySparseIndices) lives in the properties map (or, for an
		// accessor, only in getters/setters) - never in elements, so
		// arrObj.Get(i) would report it as Undefined instead of its real
		// value. arraySparseIndexValue reads either kind correctly,
		// calling the getter for an accessor index.
		for _, idx := range arraySparseIndices(arrObj, true) {
			value, err := arraySparseIndexValue(vmInstance, arrObj, obj, idx)
			if err != nil {
				return vm.Undefined, err
			}
			valuesArray.Append(value)
		}
		// Named (non-index) own properties, accessor or plain - see
		// arrayNamedKeys' own doc comment. Before this, Object.values(arr)
		// never returned a value for `arr.foo = ...` at all, regardless of
		// whether foo was a plain property or an accessor.
		for _, key := range arrayNamedKeys(arrObj, true) {
			value, err := arrayNamedKeyValue(vmInstance, arrObj, obj, key)
			if err != nil {
				return vm.Undefined, err
			}
			valuesArray.Append(value)
		}
	case vm.TypeFunction:
		funcObj := obj.AsFunction()
		if funcObj.Properties != nil {
			for _, key := range funcObj.Properties.OwnKeys() {
				if _, _, en, _, ok := funcObj.Properties.GetOwnDescriptor(key); ok && en {
					value, _ := funcObj.Properties.GetOwn(key)
					valuesArray.Append(value)
				}
			}
		}
	case vm.TypeClosure:
		closure := obj.AsClosure()
		if closure.Properties != nil {
			for _, key := range closure.Properties.OwnKeys() {
				if _, _, en, _, ok := closure.Properties.GetOwnDescriptor(key); ok && en {
					value, _ := closure.Properties.GetOwn(key)
					valuesArray.Append(value)
				}
			}
		}
	case vm.TypeBoundFunction:
		bf := obj.AsBoundFunction()
		if bf.Properties != nil {
			for _, key := range bf.Properties.OwnKeys() {
				if _, _, en, _, ok := bf.Properties.GetOwnDescriptor(key); ok && en {
					value, _ := bf.Properties.GetOwn(key)
					valuesArray.Append(value)
				}
			}
		}
	}

	return values, nil
}

// sameValue implements the ES SameValue comparison semantics
func sameValue(x, y vm.Value) bool {
	if x.Type() != y.Type() {
		// Special-case +0 and -0 for numbers: SameValue(-0, +0) is false
		if (x.Type() == vm.TypeFloatNumber || x.Type() == vm.TypeIntegerNumber) && (y.Type() == vm.TypeFloatNumber || y.Type() == vm.TypeIntegerNumber) {
			// handled below by numeric rules
		} else {
			return false
		}
	}
	switch x.Type() {
	case vm.TypeNull, vm.TypeUndefined:
		return true
	case vm.TypeBoolean:
		return x.AsBoolean() == y.AsBoolean()
	case vm.TypeString:
		return x.ToString() == y.ToString()
	case vm.TypeFloatNumber, vm.TypeIntegerNumber:
		// NaN is SameValue to NaN
		xf := x.ToFloat()
		yf := y.ToFloat()
		if math.IsNaN(xf) && math.IsNaN(yf) {
			return true
		}
		// Distinguish +0 and -0: compare reciprocals
		if xf == 0 && yf == 0 {
			return 1/xf == 1/yf
		}
		return xf == yf
	default:
		// Objects/functions: identity
		return x.Is(y)
	}
}

func objectEntriesWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined to object")
	}

	obj := args[0]
	// ECMAScript: throw TypeError for null/undefined
	if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}
	// ToObject for primitives
	if !obj.IsObject() && !obj.IsCallable() {
		converted, err := vmInstance.ToObject(obj)
		if err != nil {
			return vm.NewArray(), nil
		}
		obj = converted
	}

	entries := vm.NewArray()
	entriesArray := entries.AsArray()

	switch obj.Type() {
	case vm.TypeObject:
		plainObj := obj.AsPlainObject()
		for _, key := range plainObj.OwnKeys() {
			if _, _, en, _, ok := plainObj.GetOwnDescriptor(key); ok && en {
				value, _ := plainObj.GetOwn(key)
				entry := vm.NewArray()
				entry.AsArray().Append(vm.NewString(key))
				entry.AsArray().Append(value)
				entriesArray.Append(entry)
			}
		}
	case vm.TypeDictObject:
		dictObj := obj.AsDictObject()
		for _, key := range dictObj.OwnKeys() {
			value, _ := dictObj.GetOwn(key)
			entry := vm.NewArray()
			entry.AsArray().Append(vm.NewString(key))
			entry.AsArray().Append(value)
			entriesArray.Append(entry)
		}
	case vm.TypeArray:
		arrObj := obj.AsArray()
		for i := 0; i < arrObj.DenseLength(); i++ {
			key := strconv.Itoa(i)
			if !arrObj.HasOwnIndexProperty(key, i) {
				continue // hole - not an own property at all (paserati#300)
			}
			// arrayDenseIndexValue, not arrObj.Get(i) - see that function's
			// own doc comment for why a plain element read misses an index
			// accessor installed via Object.defineProperty.
			value, err := arrayDenseIndexValue(vmInstance, arrObj, obj, i)
			if err != nil {
				return vm.Undefined, err
			}
			entry := vm.NewArray()
			entry.AsArray().Append(vm.NewString(key))
			entry.AsArray().Append(value)
			entriesArray.Append(entry)
		}
		// A sparse index beyond the dense range (paserati#176/#178 - see
		// arraySparseIndices) lives in the properties map (or, for an
		// accessor, only in getters/setters) - never in elements, so
		// arrObj.Get(i) would report it as Undefined instead of its real
		// value. arraySparseIndexValue reads either kind correctly,
		// calling the getter for an accessor index.
		for _, idx := range arraySparseIndices(arrObj, true) {
			value, err := arraySparseIndexValue(vmInstance, arrObj, obj, idx)
			if err != nil {
				return vm.Undefined, err
			}
			entry := vm.NewArray()
			entry.AsArray().Append(vm.NewString(strconv.Itoa(idx)))
			entry.AsArray().Append(value)
			entriesArray.Append(entry)
		}
		// Named (non-index) own properties, accessor or plain - see
		// arrayNamedKeys' own doc comment. Before this, Object.entries(arr)
		// never produced an entry for `arr.foo = ...` at all, regardless of
		// whether foo was a plain property or an accessor.
		for _, key := range arrayNamedKeys(arrObj, true) {
			value, err := arrayNamedKeyValue(vmInstance, arrObj, obj, key)
			if err != nil {
				return vm.Undefined, err
			}
			entry := vm.NewArray()
			entry.AsArray().Append(vm.NewString(key))
			entry.AsArray().Append(value)
			entriesArray.Append(entry)
		}
	case vm.TypeFunction:
		funcObj := obj.AsFunction()
		if funcObj.Properties != nil {
			for _, key := range funcObj.Properties.OwnKeys() {
				if _, _, en, _, ok := funcObj.Properties.GetOwnDescriptor(key); ok && en {
					value, _ := funcObj.Properties.GetOwn(key)
					entry := vm.NewArray()
					entry.AsArray().Append(vm.NewString(key))
					entry.AsArray().Append(value)
					entriesArray.Append(entry)
				}
			}
		}
	case vm.TypeClosure:
		closure := obj.AsClosure()
		if closure.Properties != nil {
			for _, key := range closure.Properties.OwnKeys() {
				if _, _, en, _, ok := closure.Properties.GetOwnDescriptor(key); ok && en {
					value, _ := closure.Properties.GetOwn(key)
					entry := vm.NewArray()
					entry.AsArray().Append(vm.NewString(key))
					entry.AsArray().Append(value)
					entriesArray.Append(entry)
				}
			}
		}
	case vm.TypeBoundFunction:
		bf := obj.AsBoundFunction()
		if bf.Properties != nil {
			for _, key := range bf.Properties.OwnKeys() {
				if _, _, en, _, ok := bf.Properties.GetOwnDescriptor(key); ok && en {
					value, _ := bf.Properties.GetOwn(key)
					entry := vm.NewArray()
					entry.AsArray().Append(vm.NewString(key))
					entry.AsArray().Append(value)
					entriesArray.Append(entry)
				}
			}
		}
	}

	return entries, nil
}

// isIntegerIndex checks if a string represents a valid integer index (0, 1, 2, ...)
func isIntegerIndex(s string) bool {
	if s == "" {
		return false
	}
	// Leading zeros not allowed (except "0" itself)
	if len(s) > 1 && s[0] == '0' {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func objectGetOwnPropertyNamesWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined to object")
	}
	obj := args[0]

	// ECMAScript: throw TypeError for null/undefined
	if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}

	arr := vm.NewArray()
	arrObj := arr.AsArray()

	// Handle different object types
	switch obj.Type() {
	case vm.TypeString:
		// String primitives have own properties for each character index plus "length"
		s := obj.ToString()
		for i := 0; i < len(s); i++ {
			arrObj.Append(vm.NewString(strconv.Itoa(i)))
		}
		arrObj.Append(vm.NewString("length"))
		return arr, nil
	case vm.TypeObject:
		po := obj.AsPlainObject()
		// OwnPropertyNames returns ALL own string property names including non-enumerable
		for _, k := range po.OwnPropertyNames() {
			arrObj.Append(vm.NewString(k))
		}
	case vm.TypeDictObject:
		// This used to be an unreachable `else if` nested inside the
		// `case vm.TypeObject:` body above (dead code - within that case,
		// obj.Type() is always TypeObject, so the else-if branch could
		// never run) - meaning Object.getOwnPropertyNames on a DictObject
		// (a TypeScript `enum`, or a module namespace object - both
		// reachable from user code, see pkg/compiler/compile_enum.go and
		// module_bindings.go) fell all the way through to this function's
		// `default: return arr, nil` and answered [] instead of listing
		// the enum's real own properties. Found while adding this
		// function's new TypeProxy case, since a Proxy wrapping a
		// DictObject target would otherwise silently inherit the exact
		// same bug through the new delegation.
		d := obj.AsDictObject()
		// DictObject.OwnPropertyNames returns all property names
		for _, k := range d.OwnPropertyNames() {
			arrObj.Append(vm.NewString(k))
		}
	case vm.TypeArray:
		a := obj.AsArray()
		for i := 0; i < a.DenseLength(); i++ {
			key := strconv.Itoa(i)
			if !a.HasOwnIndexProperty(key, i) {
				continue // hole - not an own property at all (paserati#300)
			}
			arrObj.Append(vm.NewString(key))
		}
		// A sparse index beyond the dense range (paserati#176/#178 - see
		// arraySparseIndices) is an integer-indexed own property too, so
		// per OrdinaryOwnPropertyKeys it belongs here, in ascending numeric
		// order, before "length" and every other string key - not dropped
		// by the `!LooksLikeArrayIndex` filter below, which is for named
		// (non-index) properties only. getOwnPropertyNames wants every own
		// key regardless of enumerability, hence enumerableOnly=false.
		for _, idx := range arraySparseIndices(a, false) {
			arrObj.Append(vm.NewString(strconv.Itoa(idx)))
		}
		arrObj.Append(vm.NewString("length"))
		// Named own properties (accessor or plain) - see arrayNamedKeys'
		// own doc comment for why NamedPropertyKeys() alone missed a named
		// accessor property here. enumerableOnly=false: getOwnPropertyNames
		// wants every own key regardless of enumerability, same as the
		// sparse-index loop above.
		for _, key := range arrayNamedKeys(a, false) {
			arrObj.Append(vm.NewString(key))
		}
	case vm.TypeFunction:
		fn := obj.AsFunction()
		// Per ECMAScript OrdinaryOwnPropertyKeys:
		// 1. Integer indices in ascending numeric order
		// 2. String keys in property creation order
		// For functions: "length", "name", "prototype" are created first, then user properties
		propNames := fn.Properties.OwnPropertyNames() // Already sorted with integers first
		// Add any integer indices from properties first (already done by OwnPropertyNames)
		for _, k := range propNames {
			// Check if it's an integer index
			if isIntegerIndex(k) {
				arrObj.Append(vm.NewString(k))
			}
		}

		// Then add standard function properties
		arrObj.Append(vm.NewString("length"))
		arrObj.Append(vm.NewString("name"))

		// Check if prototype is in Properties
		hasPrototype := false
		for _, k := range propNames {
			if k == "prototype" {
				hasPrototype = true
				break
			}
		}
		if hasPrototype {
			arrObj.Append(vm.NewString("prototype"))
		}

		// Add user-defined string properties excluding built-ins and integer indices
		for _, k := range propNames {
			if k != "length" && k != "name" && k != "prototype" && !isIntegerIndex(k) {
				arrObj.Append(vm.NewString(k))
			}
		}

		// If no prototype was found in Properties, only synthesize one when
		// this function is actually constructible - an arrow function or a
		// plain (non-generator) async function has NO "prototype" own
		// property at all per spec, unlike an ordinary function, a
		// generator function, or an async generator function, which all
		// have one. This used to append "prototype" here unconditionally,
		// which was wrong for exactly those two kinds (verified against
		// Node: Object.getOwnPropertyNames(() => {}) is ["length","name"],
		// no "prototype"). vm.IsConstructor already implements this exact
		// rule for TypeFunction/TypeClosure
		// (!IsArrowFunction && !(IsAsync && !IsGenerator)) - reused here
		// rather than duplicated.
		if !hasPrototype && vmInstance.IsConstructor(obj) {
			arrObj.Append(vm.NewString("prototype"))
		}
	case vm.TypeClosure:
		cl := obj.AsClosure()
		// Per ECMAScript OrdinaryOwnPropertyKeys - check closure's own Properties first,
		// then fall back to underlying FunctionObject's Properties
		var propNames []string
		if cl.Properties != nil {
			// Per-closure properties (prototype, static methods for classes)
			propNames = cl.Properties.OwnPropertyNames()
		} else if cl.Fn != nil && cl.Fn.Properties != nil {
			// Fall back to shared function properties
			propNames = cl.Fn.Properties.OwnPropertyNames()
		}

		// Add integer indices first
		for _, k := range propNames {
			if isIntegerIndex(k) {
				arrObj.Append(vm.NewString(k))
			}
		}

		// Then standard function properties
		arrObj.Append(vm.NewString("length"))
		arrObj.Append(vm.NewString("name"))

		hasPrototype := false
		for _, k := range propNames {
			if k == "prototype" {
				hasPrototype = true
				break
			}
		}
		if hasPrototype {
			arrObj.Append(vm.NewString("prototype"))
		}

		// Add user-defined string properties
		for _, k := range propNames {
			if k != "length" && k != "name" && k != "prototype" && !isIntegerIndex(k) {
				arrObj.Append(vm.NewString(k))
			}
		}

		// Same guard as the TypeFunction case above - see its comment for
		// the full rationale and Node verification.
		if !hasPrototype && vmInstance.IsConstructor(obj) {
			arrObj.Append(vm.NewString("prototype"))
		}
	case vm.TypeNativeFunctionWithProps:
		nfp := obj.AsNativeFunctionWithProps()
		propNames := nfp.Properties.OwnPropertyNames()

		// Add integer indices first
		for _, k := range propNames {
			if isIntegerIndex(k) {
				arrObj.Append(vm.NewString(k))
			}
		}

		// Then standard function properties
		arrObj.Append(vm.NewString("length"))
		arrObj.Append(vm.NewString("name"))

		// Add remaining string properties excluding builtins and integer indices
		for _, k := range propNames {
			if k != "length" && k != "name" && !isIntegerIndex(k) {
				arrObj.Append(vm.NewString(k))
			}
		}
	case vm.TypeNativeFunction:
		// A plain (non-Props) native function - this switch never had a case
		// for it at all, so Object.getOwnPropertyNames fell to the
		// default/"non-object types" branch and answered [] even for a
		// function with real own properties (Object.defineProperty already
		// let you add one - Object.getOwnPropertyDescriptor(s) already
		// listed it correctly, PR #339). Mirrors the
		// TypeNativeFunctionWithProps case above - "length"/"name" are
		// synthesized (not real own properties for this kind either), and
		// "prototype" is included only when this native function is
		// actually a constructor (checked here since, unlike
		// TypeFunction/TypeClosure below, every TypeNativeFunction defined
		// in this codebase today is IsConstructor==false in practice, so
		// this guard is what correctly keeps "prototype" OFF a plain
		// native function like Array.prototype.slice - verified against
		// Node, which agrees: no "prototype" there).
		nf := obj.AsNativeFunction()
		var propNames []string
		if props := vm.OwnPropertiesTable(obj); props != nil {
			propNames = props.OwnPropertyNames()
		}

		for _, k := range propNames {
			if isIntegerIndex(k) {
				arrObj.Append(vm.NewString(k))
			}
		}

		arrObj.Append(vm.NewString("length"))
		arrObj.Append(vm.NewString("name"))

		hasPrototype := false
		for _, k := range propNames {
			if k == "prototype" {
				hasPrototype = true
				break
			}
		}
		if hasPrototype {
			arrObj.Append(vm.NewString("prototype"))
		}

		for _, k := range propNames {
			if k != "length" && k != "name" && k != "prototype" && !isIntegerIndex(k) {
				arrObj.Append(vm.NewString(k))
			}
		}

		if !hasPrototype && nf.IsConstructor {
			arrObj.Append(vm.NewString("prototype"))
		}
	case vm.TypeBoundFunction:
		// Another kind this switch never had a case for at all - fell to
		// the default/empty branch. Unlike every other callable kind
		// above, a bound function needs NO synthesis whatsoever: "name"
		// (bound " + original) and "length" are REAL own properties
		// written directly into bf.Properties at bind time (PR #343's
		// "Bound functions: 'name'/'length' is a real own property set at
		// bind time" precedent - re-synthesizing them here would just
		// duplicate them), and a bound function exotic object never has
		// its own "prototype" property at all, regardless of whether its
		// target is a constructor (verified against Node:
		// `Reflect.ownKeys(SomeClass.bind(null))` is `["length","name"]`,
		// no "prototype", even though SomeClass itself has one). So the
		// stored table's own names, already integer-indices-first per
		// OwnPropertyNames(), are the complete, correctly-ordered answer.
		bf := obj.AsBoundFunction()
		if bf.Properties != nil {
			for _, k := range bf.Properties.OwnPropertyNames() {
				arrObj.Append(vm.NewString(k))
			}
		}
	case vm.TypeProxy:
		// This switch never had a case for TypeProxy at all - it fell
		// through to `default: return arr, nil` and answered [] for ANY
		// Proxy, regardless of what its target actually has:
		//
		//   const target = { a: 1 };
		//   Object.getOwnPropertyNames(new Proxy(target, {})); // before: [] - Node: ["a"]
		//
		// proxyOwnPropertyKeys implements the shared ECMA-262 10.5.11
		// [[OwnPropertyKeys]] machinery (also used by
		// objectGetOwnPropertySymbolsWithVM's own new TypeProxy case below
		// and by Reflect.ownKeys, reflect_init.go) - one trap invocation
		// (or delegation) producing the full mixed string+symbol key list,
		// which each of those three callers then filters differently, per
		// spec (they all call the same internal method and filter its
		// result, rather than each doing its own separate trap
		// invocation - calling a possibly-side-effecting trap twice for
		// what should be one [[OwnPropertyKeys]] call would itself be a
		// bug).
		keys, err := proxyOwnPropertyKeys(vmInstance, obj)
		if err != nil {
			return vm.Undefined, err
		}
		for _, k := range keys {
			if k.Type() != vm.TypeSymbol {
				arrObj.Append(k)
			}
		}
	default:
		// Non-object types return empty array
		return arr, nil
	}

	return arr, nil
}

func objectGetOwnPropertySymbolsWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined to object")
	}
	obj := args[0]
	// ECMAScript: throw TypeError for null/undefined
	if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}
	// In ECMAScript, functions are objects and can have symbol properties
	// For primitives (boolean, number, string), ToObject wraps them - they have no own symbols
	if !obj.IsObject() && !obj.IsCallable() {
		return vm.NewArray(), nil
	}
	arr := vm.NewArray()
	arrObj := arr.AsArray()
	if obj.Type() == vm.TypeObject {
		po := obj.AsPlainObject()
		for _, s := range po.OwnSymbolKeys() {
			arrObj.Append(s)
		}
	} else if obj.Type() == vm.TypeArray {
		// ArrayObject already stores symbol-keyed properties
		// (GetSymbolProp/SetSymbolProp/HasOwnSymbolProp - that's how
		// `arr[sym] = v` works at all), but this function never had a
		// case for TypeArray at all, so Object.getOwnPropertySymbols on
		// an array with a real symbol property silently answered []
		// (verified against Node, which lists it):
		//
		//   const arr = [1, 2, 3];
		//   arr[Symbol("s")] = 42;
		//   Object.getOwnPropertySymbols(arr).length; // before: 0 - Node: 1
		//
		// ArrayObject.OwnSymbolKeys() (pkg/vm/value.go) is the new
		// enumerator this case needed - it didn't exist at all before
		// this fix, unlike PlainObject's own OwnSymbolKeys the TypeObject
		// case above already used.
		a := obj.AsArray()
		for _, s := range a.OwnSymbolKeys() {
			arrObj.Append(s)
		}
	} else if obj.Type() == vm.TypeFunction {
		// Functions store properties in their Properties field
		fn := obj.AsFunction()
		if fn.Properties != nil {
			for _, s := range fn.Properties.OwnSymbolKeys() {
				arrObj.Append(s)
			}
		}
	} else if obj.Type() == vm.TypeClosure {
		// Closures store properties in their Properties field
		cl := obj.AsClosure()
		if cl.Properties != nil {
			for _, s := range cl.Properties.OwnSymbolKeys() {
				arrObj.Append(s)
			}
		} else if cl.Fn.Properties != nil {
			for _, s := range cl.Fn.Properties.OwnSymbolKeys() {
				arrObj.Append(s)
			}
		}
	} else if obj.Type() == vm.TypeNativeFunction || obj.Type() == vm.TypeNativeFunctionWithProps || obj.Type() == vm.TypeBoundFunction {
		// These three callable kinds keep their own properties in the same
		// lazily-allocated *PlainObject side table shape as TypeFunction/
		// TypeClosure above (OwnPropertiesTable, pkg/vm/properties_table.go),
		// but this function never had a case for any of them at all - so
		// Object.getOwnPropertySymbols always answered [] even after
		// Object.defineProperty had just added a symbol-keyed own property,
		// even though Object.getOwnPropertyDescriptor(obj, sym) already
		// correctly reported that same property existing (and, for
		// TypeNativeFunction/TypeNativeFunctionWithProps,
		// Object.getOwnPropertyDescriptors already lists it correctly too -
		// PR #339 fixed that plural function's own equivalent gap for these
		// same three kinds without this singular-purpose function being
		// touched, which is what let this one lag behind unnoticed).
		if props := vm.OwnPropertiesTable(obj); props != nil {
			for _, s := range props.OwnSymbolKeys() {
				arrObj.Append(s)
			}
		}
	} else if obj.Type() == vm.TypeProxy {
		// Same gap, same fix, as objectGetOwnPropertyNamesWithVM's new
		// TypeProxy case above (see its comment for the full rationale) -
		// this function had no case for TypeProxy at all either, so
		// Object.getOwnPropertySymbols(new Proxy(target, {})) always
		// answered [] regardless of what symbol-keyed properties `target`
		// actually had. Filters proxyOwnPropertyKeys's shared mixed-key
		// result down to symbols, the mirror image of the string-only
		// filter in objectGetOwnPropertyNamesWithVM.
		keys, err := proxyOwnPropertyKeys(vmInstance, obj)
		if err != nil {
			return vm.Undefined, err
		}
		for _, k := range keys {
			if k.Type() == vm.TypeSymbol {
				arrObj.Append(k)
			}
		}
	}
	// DictObject does not support symbols; returns empty array
	return arr, nil
}

// proxyOwnPropertyKeys implements ECMA-262 10.5.11 [[OwnPropertyKeys]] for a
// Proxy exotic object - the single shared entry point objectGetOwnPropertyNamesWithVM,
// objectGetOwnPropertySymbolsWithVM, and Reflect.ownKeys (reflect_init.go)
// all delegate to and then filter differently, per spec (all three call the
// same internal method and filter its result - not each doing its own
// separate trap invocation, which would invoke a possibly-side-effecting
// trap more than once for what should be a single [[OwnPropertyKeys]] call).
//
// Implements spec steps 1-7 (revoked check, GetMethod(handler, "ownKeys"),
// CreateListFromArrayLike with its String|Symbol element-type restriction,
// and the unconditional "no duplicate entries" check) plus the "no trap"
// delegation (step 4's `return ? target.[[OwnPropertyKeys]]()`).
//
// Deliberately DOES NOT implement steps 8-16 (the [[Extensible]]/
// configurable-key invariant validation a well-behaved trap must satisfy) -
// a trap's raw result is returned as-is once past the checks above. This is
// right for a well-behaved trap (verified against Node: a trap that simply
// returns a different key list gets that list back verbatim) and wrong only
// for one that violates those invariants (Node throws a TypeError there;
// this returns the trap's result instead) - a real, narrower, deliberately
// deferred gap (see the follow-up chip this was flagged with) rather than
// the wrong tradeoff of delegating to the target's own keys instead, which
// would produce an equally wrong but LESS plausible-looking answer for the
// overwhelmingly common "trap just returns its own list" case.
func proxyOwnPropertyKeys(vmInstance *vm.VM, proxyVal vm.Value) ([]vm.Value, error) {
	proxy := proxyVal.AsProxy()
	if proxy.Revoked {
		return nil, vmInstance.NewTypeError("Cannot perform 'ownKeys' on a revoked Proxy")
	}
	handler := proxy.Handler()
	target := proxy.Target()

	// GetMethod(handler, "ownKeys"): an inherited trap counts, undefined/
	// null mean "no trap" - mirrors reflect_has.go's proxyReflectHas and
	// reflect_init.go's reflectProxySet/reflectProxyDefineDataProperty,
	// since proxyGetTrap (pkg/vm) is unexported and unreachable from this
	// package.
	var trap vm.Value
	var hasTrap bool
	switch handler.Type() {
	case vm.TypeObject:
		trap, hasTrap = handler.AsPlainObject().Get("ownKeys")
	case vm.TypeDictObject:
		trap, hasTrap = handler.AsDictObject().Get("ownKeys")
	}
	if !hasTrap || trap.Type() == vm.TypeUndefined || trap.Type() == vm.TypeNull {
		// No trap: delegate to target.[[OwnPropertyKeys]]() - concatenate
		// the string-key and symbol-key halves, which for a plain
		// (non-Proxy) target is exactly ECMA-262 10.1.11
		// OrdinaryOwnPropertyKeys's required order (integer indices, then
		// string keys, then symbol keys, all in creation order) - and
		// recurses correctly for a nested Proxy target via this same
		// function, through objectGetOwnPropertyNamesWithVM's own
		// TypeProxy case calling back into this one.
		namesVal, err := objectGetOwnPropertyNamesWithVM(vmInstance, []vm.Value{target})
		if err != nil {
			return nil, err
		}
		symsVal, err := objectGetOwnPropertySymbolsWithVM(vmInstance, []vm.Value{target})
		if err != nil {
			return nil, err
		}
		var keys []vm.Value
		if namesVal.Type() == vm.TypeArray {
			namesArr := namesVal.AsArray()
			for i := 0; i < namesArr.Length(); i++ {
				keys = append(keys, namesArr.Get(i))
			}
		}
		if symsVal.Type() == vm.TypeArray {
			symsArr := symsVal.AsArray()
			for i := 0; i < symsArr.Length(); i++ {
				keys = append(keys, symsArr.Get(i))
			}
		}
		return keys, nil
	}
	if !trap.IsCallable() {
		return nil, vmInstance.NewTypeError("'ownKeys' on proxy: trap is not a function")
	}

	trapResultArray, err := vmInstance.Call(trap, handler, []vm.Value{target})
	if err != nil {
		return nil, err
	}

	// CreateListFromArrayLike(trapResultArray, « String, Symbol »): accept
	// a real array (the overwhelmingly common case) via a fast path, or
	// any array-like object via .length + indexed access (mirrors the
	// CreateListFromArrayLike pattern already inlined at reflect_init.go's
	// "apply"/"construct" closures for their own argumentsList parameter).
	var rawElements []vm.Value
	if trapResultArray.Type() == vm.TypeArray {
		trapArr := trapResultArray.AsArray()
		for i := 0; i < trapArr.Length(); i++ {
			rawElements = append(rawElements, trapArr.Get(i))
		}
	} else if trapResultArray.IsObject() {
		lengthVal, err := vmInstance.GetProperty(trapResultArray, "length")
		if err != nil {
			return nil, err
		}
		length := int(lengthVal.ToFloat())
		if length < 0 {
			length = 0
		}
		for i := 0; i < length; i++ {
			val, err := vmInstance.GetProperty(trapResultArray, strconv.Itoa(i))
			if err != nil {
				return nil, err
			}
			rawElements = append(rawElements, val)
		}
	} else {
		return nil, vmInstance.NewTypeError("'ownKeys' on proxy: trap result is not an object")
	}

	// Dedup by CONTENT for a string (its `.ToString()`) and by IDENTITY for
	// a symbol (its underlying *SymbolObject pointer) - NOT by vm.Value
	// equality directly: two runtime-built TypeString values holding the
	// same text are not guaranteed to compare equal via Go's `==` on
	// vm.Value (verified: a literal "a" and a runtime-concatenated
	// "a" + "" trap result failed to dedup when keyed on the raw Value,
	// silently letting Node's genuine duplicate-entries TypeError through
	// as if the list were fine).
	type ownKeyDedupKey struct {
		str string
		sym *vm.SymbolObject
	}
	seen := make(map[ownKeyDedupKey]bool, len(rawElements))
	trapResult := make([]vm.Value, 0, len(rawElements))
	for _, el := range rawElements {
		if el.Type() != vm.TypeString && el.Type() != vm.TypeSymbol {
			return nil, vmInstance.NewTypeError(el.ToString() + " is not a valid property name")
		}
		var dedupKey ownKeyDedupKey
		if el.Type() == vm.TypeSymbol {
			dedupKey = ownKeyDedupKey{sym: el.AsSymbolObject()}
		} else {
			dedupKey = ownKeyDedupKey{str: el.ToString()}
		}
		if seen[dedupKey] {
			return nil, vmInstance.NewTypeError("'ownKeys' on proxy: trap returned duplicate entries")
		}
		seen[dedupKey] = true
		trapResult = append(trapResult, el)
	}

	return trapResult, nil
}

// reflectOwnKeysImpl returns own property keys: string names first (any enumerability), then symbols
func reflectOwnKeysImpl(args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.NewArray(), nil
	}
	obj := args[0]
	if !obj.IsObject() {
		return vm.NewArray(), nil
	}
	out := vm.NewArray()
	outArr := out.AsArray()
	if obj.Type() == vm.TypeObject {
		po := obj.AsPlainObject()
		// Strings first
		for _, k := range po.OwnKeys() {
			outArr.Append(vm.NewString(k))
		}
		// Then symbols
		for _, s := range po.OwnSymbolKeys() {
			outArr.Append(s)
		}
	} else if obj.Type() == vm.TypeDictObject {
		d := obj.AsDictObject()
		for _, k := range d.OwnKeys() {
			outArr.Append(vm.NewString(k))
		}
		// DictObject: no symbols yet
	} else if obj.Type() == vm.TypeArray {
		a := obj.AsArray()
		for i := 0; i < a.Length(); i++ {
			key := strconv.Itoa(i)
			if !a.HasOwnIndexProperty(key, i) {
				continue // hole - not an own property at all (paserati#300)
			}
			outArr.Append(vm.NewString(key))
		}
		outArr.Append(vm.NewString("length"))
	}
	return out, nil
}

// setObjectAssignTargetProperty copies one property onto Object.assign's
// target, dispatching on the target's concrete type. The three call sites in
// objectAssignWithVM's copy loop below used to each inline this same
// two-branch (TypeObject/TypeDictObject) dispatch directly; a TypeArray
// target matched neither, so Object.assign(arr, ...) silently did nothing at
// all when the target was an array (paserati#174) - a common pattern for
// tagging metadata onto a results array without a wrapper object.
//
// A numeric-index key gets a real indexed element - extending the array if
// needed, the same way arr[idx] = value already does (see the identical
// dispatch in op_setprop.go's TypeArray branch) - and anything else becomes
// a plain named property, exactly like the `.provisional` case that
// motivated this fix.
//
// This is the [[Set]] half of Object.assign's copy: CreateDataProperty-like
// for a plain data slot, but if target already has key as an own accessor
// property, [[Set]] must invoke its setter rather than clobbering it with a
// data value (paserati#274) - mirrors the own-accessor check vm.SetProperty
// makes for the same reason.
func setObjectAssignTargetProperty(vmInstance *vm.VM, target vm.Value, key string, value vm.Value) error {
	switch target.Type() {
	case vm.TypeObject:
		plainTarget := target.AsPlainObject()
		if _, setter, _, _, isAccessor := plainTarget.GetOwnAccessor(key); isAccessor {
			if setter.Type() == vm.TypeUndefined {
				return nil // accessor with no setter: [[Set]] silently no-ops (non-strict)
			}
			_, err := vmInstance.Call(setter, target, []vm.Value{value})
			return err
		}
		// Object.assign copies as if by ordinary [[Set]] - the property must
		// land enumerable on the target, not non-enumerable (paserati#168).
		// SetOwnNonEnumerable exists for built-in method registration, not
		// for this.
		plainTarget.SetOwn(key, value)
	case vm.TypeDictObject:
		target.AsDictObject().SetOwn(key, value)
	case vm.TypeArray:
		arr := target.AsArray()
		if idx, isIndex := vm.ParseArrayIndex(key); isIndex {
			// ArrayObject.Set is O(idx): it fills every slot up to idx with
			// Hole before writing. ParseArrayIndex accepts anything up to
			// 2^32-2, so an object source key like "4294967294" would try
			// to allocate/loop over four billion slots and hang - the same
			// hazard maxDenseArraySetIndex already guards against for
			// arrayLikeSet (array_generic.go) and maxDenseArrayDefineIndex
			// guards for ArrayDefineOwnProperty (package vm). Past the
			// bound, track it as a named/sparse property instead, same
			// tradeoff those two call sites make.
			if idx <= maxDenseArraySetIndex {
				arr.Set(idx, value)
			} else {
				arr.DefineOwnProperty(key, value, true, true, true)
				if idx+1 > arr.Length() {
					arr.SetLength(idx + 1)
				}
			}
		} else {
			arr.SetOwn(key, value)
		}
	case vm.TypeFunction, vm.TypeClosure, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction:
		// Functions (plain, closures, natives, and bound functions) are
		// ordinary callable objects to user code - Object.assign onto one
		// used to match none of the branches above and silently drop every
		// source property (paserati#254; the actual construction pattern
		// @babel/template's public API uses: bind the callable, then
		// Object.assign named sub-builders onto it). Their own properties
		// live in a side table reached via EnsureOwnPropertiesTable, which
		// allocates the table lazily the same way direct assignment
		// (`fn.a = 1`) already does for these types - and lands the
		// property enumerable, just like SetOwn does for TypeObject above.
		if props := vm.EnsureOwnPropertiesTable(target); props != nil {
			props.SetOwn(key, value)
		}
	}
	return nil
}

// setObjectAssignTargetPropertyByKey is setObjectAssignTargetProperty for a
// symbol key - backing Object.assign's own symbol-key copy loops below
// (previously nonexistent: no source branch ever walked a symbol key at
// all, so there was nothing to write here either - see objectAssignWithVM's
// doc comment). Mirrors the string-key version's exact per-target-kind
// scope: an accessor is only checked for a TypeObject target (the
// string-key version doesn't check one for TypeArray or the callable
// side-table kinds either - a separate, narrower, pre-existing limitation
// this function deliberately doesn't widen).
//
// There is no PlainObject.SetOwnByKey (only the string-keyed SetOwn, which
// itself implements "preserve existing writable/enumerable/configurable,
// default a brand-new key to true/true/true"), so the TypeObject and
// callable-side-table branches reproduce that same rule explicitly via
// HasOwnByKey + DefineOwnPropertyByKey, matching vm.setOwnCheckedByKey's
// identical pattern (pkg/vm/properties_table.go) for the same "ordinary
// [[Set]], not Object.defineProperty" distinction that function's own doc
// comment explains - and the identical pattern this session's Reflect.set
// symbol-key fix (reflectCreateOrUpdateDataPropertyByKey) already used for
// the same reason.
func setObjectAssignTargetPropertyByKey(vmInstance *vm.VM, target vm.Value, sym vm.Value, value vm.Value) error {
	key := vm.NewSymbolKey(sym)
	switch target.Type() {
	case vm.TypeObject:
		plainTarget := target.AsPlainObject()
		if _, setter, _, _, isAccessor := plainTarget.GetOwnAccessorByKey(key); isAccessor {
			if setter.Type() == vm.TypeUndefined {
				return nil // accessor with no setter: [[Set]] silently no-ops (non-strict)
			}
			_, err := vmInstance.Call(setter, target, []vm.Value{value})
			return err
		}
		if plainTarget.HasOwnByKey(key) {
			plainTarget.DefineOwnPropertyByKey(key, value, nil, nil, nil)
		} else {
			w, e, c := true, true, true
			plainTarget.DefineOwnPropertyByKey(key, value, &w, &e, &c)
		}
	case vm.TypeDictObject:
		// DictObjects have no symbol-keyed storage at all - matches every
		// other DictObject-and-symbols case in this codebase.
	case vm.TypeArray:
		if symObj := sym.AsSymbolObject(); symObj != nil {
			target.AsArray().SetSymbolProp(symObj, value)
		}
	case vm.TypeFunction, vm.TypeClosure, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction:
		if props := vm.EnsureOwnPropertiesTable(target); props != nil {
			if props.HasOwnByKey(key) {
				props.DefineOwnPropertyByKey(key, value, nil, nil, nil)
			} else {
				w, e, c := true, true, true
				props.DefineOwnPropertyByKey(key, value, &w, &e, &c)
			}
		}
	}
	return nil
}

func objectAssignWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}

	// First argument is the target
	target := args[0]

	// Convert primitives to objects (except null/undefined which throw)
	if target.Type() == vm.TypeNull || target.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}

	// Box primitive targets to objects
	if !target.IsObject() {
		// Box primitives: Number, String, Boolean, Symbol
		switch target.Type() {
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			// Box to Number object - but Object.assign returns the boxed object
			target = vmInstance.NewNumberObject(target.ToFloat())
		case vm.TypeString:
			target = vmInstance.NewStringObject(target.ToString())
		case vm.TypeBoolean:
			// Box to Boolean object (we'd need to add NewBooleanObject)
			// For now, create a plain object with [[PrimitiveValue]]
			obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			obj.SetOwnNonEnumerable("[[PrimitiveValue]]", target)
			target = vm.NewValueFromPlainObject(obj)
		case vm.TypeSymbol:
			// Box to Symbol object
			obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			obj.SetOwnNonEnumerable("[[PrimitiveValue]]", target)
			target = vm.NewValueFromPlainObject(obj)
		}
	}

	// Copy properties from all source objects
	for i := 1; i < len(args); i++ {
		source := args[i]

		// Skip null and undefined sources
		if source.Type() == vm.TypeNull || source.Type() == vm.TypeUndefined {
			continue
		}

		// Get own enumerable properties from source
		if source.Type() == vm.TypeObject {
			plainObj := source.AsPlainObject()
			for _, key := range plainObj.OwnKeys() {
				// Object.assign reads each source property via [[Get]], which
				// for an accessor property means calling its getter rather
				// than copying the (unset) data slot underneath it
				// (paserati#274).
				var value vm.Value
				if getter, _, _, _, isAccessor := plainObj.GetOwnAccessor(key); isAccessor {
					if getter.Type() == vm.TypeUndefined {
						value = vm.Undefined
					} else {
						var err error
						value, err = vmInstance.Call(getter, source, nil)
						if err != nil {
							return vm.Undefined, err
						}
					}
				} else {
					value, _ = plainObj.GetOwn(key)
				}
				if err := setObjectAssignTargetProperty(vmInstance, target, key, value); err != nil {
					return vm.Undefined, err
				}
			}
			// Symbol-keyed own properties - this loop never existed at all,
			// so Object.assign silently dropped every symbol property a
			// TypeObject source had, plain or accessor, regardless of
			// enumerability (verified against Node, which copies an
			// enumerable one and skips a non-enumerable one, exactly like
			// the string-key loop just above):
			//
			//   const o = {}; const s = Symbol("x"); o[s] = "v";
			//   Object.assign({}, o)[s]; // before: undefined - Node: "v"
			//
			// Same accessor-invokes-getter rule as the string-key loop
			// (paserati#274) - OwnSymbolKeys() (unlike OwnKeys()) isn't
			// pre-filtered to enumerable-only, since a symbol key can only
			// become non-enumerable via an explicit Object.defineProperty,
			// which is comparatively rare - so this filters explicitly per
			// key instead.
			for _, symVal := range plainObj.OwnSymbolKeys() {
				symKey := vm.NewSymbolKey(symVal)
				var value vm.Value
				if getter, _, enumerable, _, isAccessor := plainObj.GetOwnAccessorByKey(symKey); isAccessor {
					if !enumerable {
						continue
					}
					if getter.Type() == vm.TypeUndefined {
						value = vm.Undefined
					} else {
						var err error
						value, err = vmInstance.Call(getter, source, nil)
						if err != nil {
							return vm.Undefined, err
						}
					}
				} else {
					v, _, enumerable, _, ok := plainObj.GetOwnDescriptorByKey(symKey)
					if !ok || !enumerable {
						continue
					}
					value = v
				}
				if err := setObjectAssignTargetPropertyByKey(vmInstance, target, symVal, value); err != nil {
					return vm.Undefined, err
				}
			}
		} else if source.Type() == vm.TypeDictObject {
			dictObj := source.AsDictObject()
			for _, key := range dictObj.OwnKeys() {
				value, _ := dictObj.GetOwn(key)
				if err := setObjectAssignTargetProperty(vmInstance, target, key, value); err != nil {
					return vm.Undefined, err
				}
			}
		} else if source.Type() == vm.TypeArray {
			arrObj := source.AsArray()
			// For arrays, copy indexed properties. arrayDenseIndexValue, not
			// arrObj.Get(i): HasOwnIndexProperty above correctly treats an
			// index accessor (installed via Object.defineProperty) as
			// existing, but a bare arrObj.Get(i) reads `elements` directly
			// with no accessor check, so it would copy the stale
			// underlying element instead of calling the getter - see
			// arrayDenseIndexValue's own doc comment.
			for i := 0; i < arrObj.DenseLength(); i++ {
				key := strconv.Itoa(i)
				if !arrObj.HasOwnIndexProperty(key, i) {
					continue // hole - not an own property at all, nothing to copy (paserati#300)
				}
				value, err := arrayDenseIndexValue(vmInstance, arrObj, source, i)
				if err != nil {
					return vm.Undefined, err
				}
				if err := setObjectAssignTargetProperty(vmInstance, target, key, value); err != nil {
					return vm.Undefined, err
				}
			}
			// A sparse index beyond the dense range (paserati#176/#178 -
			// see arraySparseIndices) lives in the properties map (or, for
			// an accessor, only in getters/setters) - never in elements,
			// so arrObj.Get(i) would report it as Undefined instead of
			// copying its real value. arraySparseIndexValue reads either
			// kind correctly, calling the getter for an accessor index
			// (per spec, Object.assign copies a source's own enumerable
			// properties via [[Get]], which for an accessor means calling
			// it - same reasoning as paserati#274, already applied to the
			// TypeObject source branch above). Object.assign only reads a
			// source's own ENUMERABLE properties, hence enumerableOnly=true.
			for _, idx := range arraySparseIndices(arrObj, true) {
				value, err := arraySparseIndexValue(vmInstance, arrObj, source, idx)
				if err != nil {
					return vm.Undefined, err
				}
				key := strconv.Itoa(idx)
				if err := setObjectAssignTargetProperty(vmInstance, target, key, value); err != nil {
					return vm.Undefined, err
				}
			}
			// Named (non-index) accessor properties, e.g.
			// Object.defineProperty(arr, "foo", {get, set, enumerable}) -
			// stored in getters/setters, never in `properties` (see
			// ArrayObject.DefineAccessorProperty's own doc comment), so the
			// named-data loop just below can't see these at all. This whole
			// branch - named string properties on an array source, accessor
			// or plain - never existed prior to this fix: Object.assign(
			// {}, arr) only ever copied indexed elements, silently dropping
			// anything set via `arr.foo = ...` or Object.defineProperty:
			//
			//   const arr = [1, 2]; arr.foo = "bar";
			//   Object.assign({}, arr).foo; // before: undefined - Node: "bar"
			//
			// AccessorKeys() can also report a numeric-index key (an
			// accessor installed AT an array index via Object.defineProperty
			// - ParseArrayIndex skips those here since the dense/sparse
			// index loops above already own that key space (though neither
			// of those loops actually invokes an index accessor's getter
			// today - a separate, narrower, pre-existing gap not touched by
			// this fix; see arraySparseIndexValue's own accessor handling
			// for the sparse-index case, which DOES get this right - the
			// gap is specific to the DENSE-range loop above, whose
			// arrObj.Get(i) reads straight from `elements` with no accessor
			// check at all). Deliberately vm.ParseArrayIndex, NOT
			// vm.LooksLikeArrayIndex: the latter has no upper bound, so a key
			// like "4294967295" (past the 2^32-2 array-index ceiling) would
			// look like an index here and get skipped, while arraySparseIndices'
			// own vm.ParseArrayIndex-based filter (used below) also rejects it
			// as too big - the two filters must use the SAME predicate or a
			// key can fall in the gap between them and vanish entirely.
			for _, name := range arrObj.AccessorKeys() {
				if _, isIndex := vm.ParseArrayIndex(name); isIndex {
					continue
				}
				getter, _, enumerable, _, isAccessor := arrObj.GetOwnAccessor(name)
				if !isAccessor || !enumerable {
					continue
				}
				var value vm.Value
				if getter.Type() == vm.TypeUndefined {
					value = vm.Undefined
				} else {
					var err error
					value, err = vmInstance.Call(getter, source, nil)
					if err != nil {
						return vm.Undefined, err
					}
				}
				if err := setObjectAssignTargetProperty(vmInstance, target, name, value); err != nil {
					return vm.Undefined, err
				}
			}
			// Named (non-index) plain data properties, e.g. `arr.foo = "bar"`.
			// NamedPropertyKeys() (despite its doc comment) also returns any
			// sparse-index key sharing the same `properties` map - already
			// handled above via arraySparseIndices - so vm.ParseArrayIndex
			// filters those back out here, the exact same predicate
			// arraySparseIndices itself uses to find only the ones that ARE
			// indices (not vm.LooksLikeArrayIndex, which has no upper bound
			// and would leave an out-of-range numeric key like
			// "4294967295" matched by neither filter - see the AccessorKeys
			// loop above for the full explanation).
			for _, name := range arrObj.NamedPropertyKeys() {
				if _, isIndex := vm.ParseArrayIndex(name); isIndex {
					continue
				}
				value, enumerable, ok := arrObj.GetNamedPropertyDescriptor(name)
				if !ok || !enumerable {
					continue
				}
				if err := setObjectAssignTargetProperty(vmInstance, target, name, value); err != nil {
					return vm.Undefined, err
				}
			}
			// Symbol-keyed properties, plain or accessor (see
			// ArrayDefineOwnSymbolProperty, pkg/vm/array_props.go) - same
			// gap and same fix shape as the TypeObject source branch's own
			// symbol loop above, just against ArrayObject's own symbol
			// storage (GetOwnSymbolAccessor/GetSymbolPropertyDescriptor)
			// instead of PlainObject's.
			for _, symVal := range arrObj.OwnSymbolKeys() {
				symObj := symVal.AsSymbolObject()
				var value vm.Value
				if getter, _, enumerable, _, isAccessor := arrObj.GetOwnSymbolAccessor(symObj); isAccessor {
					if !enumerable {
						continue
					}
					if getter.Type() == vm.TypeUndefined {
						value = vm.Undefined
					} else {
						var err error
						value, err = vmInstance.Call(getter, source, nil)
						if err != nil {
							return vm.Undefined, err
						}
					}
				} else {
					v, desc, ok := arrObj.GetSymbolPropertyDescriptor(symObj)
					if !ok || !desc.Enumerable {
						continue
					}
					value = v
				}
				if err := setObjectAssignTargetPropertyByKey(vmInstance, target, symVal, value); err != nil {
					return vm.Undefined, err
				}
			}
			// Also copy length property. Deliberately no vm.TypeArray case
			// here (unlike setObjectAssignTargetProperty above, which now
			// handles all three target types uniformly): an array's own
			// "length" isn't enumerable, so real Object.assign wouldn't
			// copy it at all - copying it here for object/dict targets is
			// itself a pre-existing spec deviation (see paserati#168), and
			// extending it to array targets would overwrite (truncate or
			// grow) the target's own length instead of adding a property,
			// which is worse than the existing deviation, not a fix for it.
			if target.Type() == vm.TypeObject {
				targetPlain := target.AsPlainObject()
				targetPlain.SetOwnNonEnumerable("length", vm.NumberValue(float64(arrObj.Length())))
			} else if target.Type() == vm.TypeDictObject {
				targetDict := target.AsDictObject()
				targetDict.SetOwn("length", vm.NumberValue(float64(arrObj.Length())))
			}
		}
	}

	return target, nil
}

func objectHasOwnWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) < 2 {
		return vm.Undefined, vmInstance.NewTypeError("Object.hasOwn requires 2 arguments")
	}

	// Step 1: ToObject(O)
	obj := args[0]
	if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}

	// Step 2: ToPropertyKey(P) - this may call valueOf/toString
	keyVal := args[1]

	// For objects/callables, call ToPrimitive with "string" hint to get the property key
	if keyVal.IsObject() || keyVal.IsCallable() {
		vmInstance.EnterHelperCall()
		primitiveVal := vmInstance.ToPrimitive(keyVal, "string")
		vmInstance.ExitHelperCall()

		// Check if ToPrimitive threw an exception
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return vm.Undefined, nil // Let exception propagate
		}
		keyVal = primitiveVal
	}

	// Now keyVal is either a Symbol or can be converted to string
	isSymbol := keyVal.Type() == vm.TypeSymbol

	// Check if object has the property as own property
	if obj.Type() == vm.TypeObject {
		plainObj := obj.AsPlainObject()
		if isSymbol {
			return vm.BooleanValue(plainObj.HasOwnByKey(vm.NewSymbolKey(keyVal))), nil
		}
		_, hasOwn := plainObj.GetOwn(keyVal.ToString())
		return vm.BooleanValue(hasOwn), nil
	}
	if obj.Type() == vm.TypeDictObject {
		dictObj := obj.AsDictObject()
		if isSymbol {
			return vm.BooleanValue(false), nil
		}
		_, hasOwn := dictObj.GetOwn(keyVal.ToString())
		return vm.BooleanValue(hasOwn), nil
	}
	if obj.Type() == vm.TypeArray {
		arrObj := obj.AsArray()
		if isSymbol {
			return vm.BooleanValue(false), nil
		}
		propName := keyVal.ToString()
		// For arrays, check if it's a valid index or 'length'
		if propName == "length" {
			return vm.BooleanValue(true), nil
		}
		// Check numeric indices
		if index, err := strconv.Atoi(propName); err == nil && index >= 0 {
			return vm.BooleanValue(arrObj.HasOwnIndexProperty(propName, index)), nil
		}
	}

	return vm.BooleanValue(false), nil
}

func objectFromEntriesImpl(args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		// TODO: Throw TypeError when error objects are implemented
		// Create an empty plain object (this should use the Object prototype)
		return vm.NewObject(vm.Undefined), nil
	}

	iterable := args[0]

	// Create new object to populate (use undefined to get Object.prototype)
	result := vm.NewObject(vm.Undefined)
	resultObj := result.AsPlainObject()

	// If it's an array, iterate through it
	if iterable.Type() == vm.TypeArray {
		arr := iterable.AsArray()
		for i := 0; i < arr.Length(); i++ {
			entry := arr.Get(i)

			// Each entry should be an array-like with at least 2 elements
			if entry.Type() == vm.TypeArray {
				entryArr := entry.AsArray()
				if entryArr.Length() >= 2 {
					key := entryArr.Get(0).ToString()
					value := entryArr.Get(1)
					resultObj.SetOwnNonEnumerable(key, value)
				}
			}
		}
	}
	// TODO: Support other iterables when iterator protocol is implemented

	return result, nil
}

// definePropertyTarget returns the PlainObject that objectDefinePropertyWithVM
// will ultimately define on: the object itself, or the side table for a
// callable / RegExp / Map / Set. Returns nil for kinds with their own define
// logic (arrays, typed arrays, dicts, proxies).
func definePropertyTarget(vmInstance *vm.VM, obj vm.Value) *vm.PlainObject {
	if obj.Type() == vm.TypeObject {
		return obj.AsPlainObject()
	}
	return vm.OwnPropertiesTable(obj)
}

// definePropertyRejected builds the TypeError that DefinePropertyOrThrow
// (ES2025 7.3.8) raises when [[DefineOwnProperty]] returns false - a redefinition
// the property's current attributes don't permit. Reflect.defineProperty turns
// this back into a false return; Object.defineProperty lets it throw.
func definePropertyRejected(vmInstance *vm.VM, propName string, propSym vm.Value, keyIsSymbol bool) error {
	key := propName
	if keyIsSymbol {
		key = propSym.ToString()
	}
	return vmInstance.NewTypeError("Cannot redefine property: " + key)
}

func objectDefinePropertyWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) < 3 {
		return vm.Undefined, vmInstance.NewTypeError("Object.defineProperty requires 3 arguments")
	}

	obj := args[0]
	// Property key: support symbols natively, and use ToPrimitive for objects (ToPropertyKey)
	var keyIsSymbol bool
	var propName string
	var propSym vm.Value
	keyArg := args[1]
	if keyArg.Type() == vm.TypeSymbol {
		keyIsSymbol = true
		propSym = keyArg
	} else {
		// ToPropertyKey: for objects, call ToPrimitive with "string" hint first
		if keyArg.IsObject() || keyArg.IsCallable() {
			primKey := vmInstance.ToPrimitive(keyArg, "string")
			if primKey.Type() == vm.TypeSymbol {
				keyIsSymbol = true
				propSym = primKey
			} else {
				propName = primKey.ToString()
			}
		} else {
			propName = keyArg.ToString()
		}
	}
	descriptor := args[2]

	// Handle Proxy objects
	if obj.Type() == vm.TypeProxy {
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot define property on revoked Proxy")
		}

		// Check for defineProperty trap. GetMethod(handler, "defineProperty")
		// per spec: an inherited trap counts, not just an own one -
		// vmInstance.ProxyGetTrap (not a bare
		// proxy.Handler().AsPlainObject().GetOwn("defineProperty")) for
		// the same reason documented on its pkg/vm definition.
		if defineTrap, ok := vmInstance.ProxyGetTrap(proxy.Handler(), "defineProperty"); ok && !defineTrap.IsUndefined() && defineTrap.Type() != vm.TypeNull {
			// Validate trap is callable
			if !defineTrap.IsFunction() {
				return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap is not a function")
			}

			// Convert property key to appropriate value
			var propKey vm.Value
			if keyIsSymbol {
				propKey = propSym
			} else {
				propKey = vm.NewString(propName)
			}

			// Call handler.defineProperty(target, property, descriptor)
			trapArgs := []vm.Value{proxy.Target(), propKey, descriptor}
			result, err := vmInstance.Call(defineTrap, proxy.Handler(), trapArgs)
			if err != nil {
				return vm.Undefined, err
			}

			// Result should be truthy to indicate success
			if result.IsFalsey() {
				return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap returned falsish")
			}

			// ECMAScript 10.5.6 steps 14-20: Validate invariants
			target := proxy.Target()

			// Step 14: Determine if descriptor sets configurable to false
			settingConfigFalse := false
			descObj := descriptor.AsPlainObject()
			if descObj != nil {
				if confVal, hasConf := descObj.GetOwn("configurable"); hasConf {
					settingConfigFalse = confVal.IsFalsey()
				}
			}

			// Step 15: Let targetDesc be target.[[GetOwnProperty]](P)
			// NOTE: Must re-read after trap, since trap may have modified target
			var targetDescFound bool
			var targetValue vm.Value
			var targetConfigurable, targetWritable, targetEnumerable bool
			if target.Type() == vm.TypeObject {
				targetObj := target.AsPlainObject()
				var found bool
				targetValue, targetWritable, targetEnumerable, targetConfigurable, found = targetObj.GetOwnDescriptor(propName)
				targetDescFound = found
			}

			// Step 16: Let extensibleTarget be target.[[IsExtensible]]()
			targetExtensible := true
			if target.Type() == vm.TypeObject {
				targetObj := target.AsPlainObject()
				targetExtensible = targetObj.IsExtensible()
			}

			if !targetDescFound {
				// Step 19: targetDesc is undefined
				// 19a: If target is not extensible, throw TypeError
				if !targetExtensible {
					return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap returned truish for adding property to a non-extensible target")
				}
				// 19b: If settingConfigFalse, throw TypeError
				if settingConfigFalse {
					return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap returned truish for defining non-configurable property which does not exist on the target")
				}
			} else {
				// Step 20: targetDesc is defined
				// 20a: IsCompatiblePropertyDescriptor check
				if !targetConfigurable {
					// Non-configurable target property - validate compatibility
					if descObj != nil {
						// Can't change configurable to true
						if confVal, hasConf := descObj.GetOwn("configurable"); hasConf && confVal.IsTruthy() {
							return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap returned truish for defining non-configurable property as configurable")
						}
						// Can't change enumerable
						if enumVal, hasEnum := descObj.GetOwn("enumerable"); hasEnum {
							if enumVal.IsTruthy() != targetEnumerable {
								return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap returned truish for incompatible property descriptor")
							}
						}
						// For non-configurable, non-writable data property, can't change value
						if !targetWritable {
							if valField, hasVal := descObj.GetOwn("value"); hasVal {
								if !valField.StrictlyEquals(targetValue) {
									return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap returned truish for incompatible property descriptor")
								}
							}
							// Can't change writable to true either
							if writableVal, hasW := descObj.GetOwn("writable"); hasW && writableVal.IsTruthy() {
								return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap returned truish for incompatible property descriptor")
							}
						}
					}
				}
				// 20b: If settingConfigFalse and target property is configurable, throw TypeError
				if settingConfigFalse && targetConfigurable {
					return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap returned truish for defining non-configurable property which is configurable on the target")
				}
				// 20c (ES2020+): If target is non-configurable data property and writable, descriptor sets writable to false → TypeError
				if !targetConfigurable && targetWritable {
					if descObj != nil {
						if writableVal, hasWritable := descObj.GetOwn("writable"); hasWritable && writableVal.IsFalsey() {
							return vm.Undefined, vmInstance.NewTypeError("'defineProperty' on proxy: trap returned truish for making non-configurable writable property non-writable")
						}
					}
				}
			}

			return obj, nil
		}

		// No trap, delegate to target
		return objectDefinePropertyWithVM(vmInstance, []vm.Value{proxy.Target(), args[1], descriptor})
	}

	// First argument must be an object (including functions, which are objects in JS)
	isObjectLike := obj.IsObject() ||
		obj.Type() == vm.TypeFunction ||
		obj.Type() == vm.TypeClosure ||
		obj.Type() == vm.TypeNativeFunctionWithProps ||
		obj.Type() == vm.TypeBoundFunction ||
		obj.Type() == vm.TypeNativeFunction
	if !isObjectLike {
		return vm.Undefined, vmInstance.NewTypeError("Object.defineProperty called on non-object")
	}

	// Module Namespace Exotic Object [[DefineOwnProperty]] behavior (ECMAScript 10.4.6.7)
	// Returns true if no change is requested, false otherwise
	if obj.Type() == vm.TypeObject {
		if plainObj := obj.AsPlainObject(); plainObj != nil && plainObj.IsModuleNamespace() {
			// Get current property descriptor
			var currentDesc vm.Value
			if keyIsSymbol {
				if _, _, wr, en, conf := plainObj.GetOwnDescriptorByKey(vm.NewSymbolKey(propSym)); wr || en || conf {
					// Property exists
					currentDesc = vm.NewObject(vmInstance.ObjectPrototype)
				}
			} else {
				if _, exists := plainObj.GetOwn(propName); exists {
					// Property exists
					currentDesc = vm.NewObject(vmInstance.ObjectPrototype)
				}
			}

			// If property doesn't exist, fail
			if currentDesc.Type() == vm.TypeUndefined {
				return vm.Undefined, vmInstance.NewTypeError("Cannot define property " + propName + " on a module namespace object")
			}

			// Property exists - check if descriptor requests any changes
			// For namespace properties, we only allow descriptors that don't change anything
			descObj := descriptor.AsPlainObject()
			if descObj != nil {
				// Check for value change
				if val, hasValue := descObj.GetOwn("value"); hasValue {
					if keyIsSymbol {
						if currentVal, ok := plainObj.GetOwnByKey(vm.NewSymbolKey(propSym)); ok {
							if !val.StrictlyEquals(currentVal) {
								return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property " + propName + " on a module namespace object")
							}
						}
					} else {
						if currentVal, ok := plainObj.GetOwn(propName); ok {
							if !val.StrictlyEquals(currentVal) {
								return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property " + propName + " on a module namespace object")
							}
						}
					}
				}
				// Check for configurable change (namespace props are always non-configurable)
				if conf, hasConf := descObj.GetOwn("configurable"); hasConf {
					if conf.IsTruthy() {
						return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property " + propName + " on a module namespace object")
					}
				}
			}

			// No changes requested or descriptor matches - return the object
			return obj, nil
		}
	}

	// Per ECMAScript 8.10.5 ToPropertyDescriptor step 1: If Type(Obj) is not Object, throw TypeError
	// Check if descriptor is an object (including functions, which are objects in JS)
	descIsObject := descriptor.IsObject() ||
		descriptor.Type() == vm.TypeFunction ||
		descriptor.Type() == vm.TypeClosure ||
		descriptor.Type() == vm.TypeNativeFunction ||
		descriptor.Type() == vm.TypeNativeFunctionWithProps ||
		descriptor.Type() == vm.TypeBoundFunction
	if !descIsObject {
		return vm.Undefined, vmInstance.NewTypeError("Property description must be an object")
	}

	// Parse descriptor object fields: value, writable, enumerable, configurable, get, set
	// Per ECMAScript 8.10.5 ToPropertyDescriptor, we use [[Get]] which follows prototype chain
	// and properly invokes accessor getters when reading descriptor properties
	var value vm.Value = vm.Undefined
	var writablePtr, enumerablePtr, configurablePtr *bool
	var getter vm.Value = vm.Undefined
	var setter vm.Value = vm.Undefined
	hasValue := false
	hasWritable := false
	hasGetter := false
	hasSetter := false

	// Helper to check if property exists and get its value using GetProperty (calls getters)
	// Per ECMAScript spec, this uses [[HasProperty]] (which checks prototype chain) and [[Get]]
	hasAndGetProperty := func(obj vm.Value, propName string) (vm.Value, bool, error) {
		// Check if property exists (including prototype chain)
		// Note: Must check type BEFORE calling AsXxx() methods which panic on wrong type
		var exists bool

		// Helper to check Function.prototype for function types
		checkFunctionPrototype := func() bool {
			if vmInstance.FunctionPrototype.Type() == vm.TypeNativeFunctionWithProps {
				nfp := vmInstance.FunctionPrototype.AsNativeFunctionWithProps()
				if nfp != nil && nfp.Properties != nil {
					return nfp.Properties.Has(propName)
				}
			}
			return false
		}

		switch obj.Type() {
		case vm.TypeObject:
			if obj.Type() == vm.TypeObject {
				po := obj.AsPlainObject()
				exists = po.Has(propName)
			}
		case vm.TypeDictObject:
			if obj.Type() == vm.TypeDictObject {
				do := obj.AsDictObject()
				_, exists = do.Get(propName)
			}
		case vm.TypeFunction:
			fn := obj.AsFunction()
			if fn != nil {
				if fn.Properties != nil && fn.Properties.Has(propName) {
					exists = true
				} else {
					// Check Function.prototype
					exists = checkFunctionPrototype()
				}
			}
		case vm.TypeClosure:
			cl := obj.AsClosure()
			if cl != nil {
				if cl.Properties != nil && cl.Properties.Has(propName) {
					exists = true
				} else {
					// Check Function.prototype
					exists = checkFunctionPrototype()
				}
			}
		case vm.TypeBoundFunction:
			bf := obj.AsBoundFunction()
			if bf != nil {
				if bf.Properties != nil && bf.Properties.Has(propName) {
					exists = true
				} else {
					// Check Function.prototype
					exists = checkFunctionPrototype()
				}
			}
		case vm.TypeNativeFunctionWithProps:
			nfp := obj.AsNativeFunctionWithProps()
			if nfp != nil {
				if nfp.Properties != nil && nfp.Properties.Has(propName) {
					exists = true
				} else {
					// Check Function.prototype
					exists = checkFunctionPrototype()
				}
			}
		case vm.TypeRegExp:
			// RegExp objects: check own properties and RegExp.prototype
			regex := obj.AsRegExpObject()
			if regex != nil {
				if regex.Properties != nil && regex.Properties.Has(propName) {
					exists = true
				} else if vmInstance.RegExpPrototype.IsObject() {
					proto := vmInstance.RegExpPrototype.AsPlainObject()
					exists = proto.Has(propName)
				}
			}
		case vm.TypeArray:
			// Array objects: check own properties and Array.prototype
			arr := obj.AsArray()
			if arr != nil {
				if _, ok := arr.GetOwn(propName); ok {
					exists = true
				} else if vmInstance.ArrayPrototype.IsObject() {
					proto := vmInstance.ArrayPrototype.AsPlainObject()
					exists = proto.Has(propName)
				}
			}
		case vm.TypeArguments:
			// Arguments objects: check own properties and Object.prototype
			args := obj.AsArguments()
			if args != nil {
				if args.HasNamedProp(propName) {
					exists = true
				} else if vmInstance.ObjectPrototype.IsObject() {
					// Check Object.prototype for inherited properties (per spec 8.10.5)
					proto := vmInstance.ObjectPrototype.AsPlainObject()
					exists = proto.Has(propName)
				}
			}
		case vm.TypeProxy:
			// Proxy objects: use the "has" trap or target
			// For simplicity, just try to get the property and see if it's defined
			val, err := vmInstance.GetProperty(obj, propName)
			if err != nil {
				return vm.Undefined, false, err
			}
			// If GetProperty returns a value that's not undefined, consider it exists
			// This is a simplification - proper proxy handling would use the "has" trap
			if val.Type() != vm.TypeUndefined {
				return val, true, nil
			}
			exists = false
		}
		if !exists {
			return vm.Undefined, false, nil
		}
		// Property exists, use GetProperty to call getters
		val, err := vmInstance.GetProperty(obj, propName)
		if err != nil {
			return vm.Undefined, false, err
		}
		return val, true, nil
	}

	// Get each descriptor field using GetProperty (calls getters per spec)
	if val, exists, err := hasAndGetProperty(descriptor, "value"); err != nil {
		return vm.Undefined, err
	} else if exists {
		hasValue = true
		value = val
	}
	if w, exists, err := hasAndGetProperty(descriptor, "writable"); err != nil {
		return vm.Undefined, err
	} else if exists {
		hasWritable = true
		b := w.IsTruthy()
		writablePtr = &b
	}
	if e, exists, err := hasAndGetProperty(descriptor, "enumerable"); err != nil {
		return vm.Undefined, err
	} else if exists {
		b := e.IsTruthy()
		enumerablePtr = &b
	}
	if c, exists, err := hasAndGetProperty(descriptor, "configurable"); err != nil {
		return vm.Undefined, err
	} else if exists {
		b := c.IsTruthy()
		configurablePtr = &b
	}
	if g, exists, err := hasAndGetProperty(descriptor, "get"); err != nil {
		return vm.Undefined, err
	} else if exists {
		hasGetter = true
		getter = g
	}
	if s, exists, err := hasAndGetProperty(descriptor, "set"); err != nil {
		return vm.Undefined, err
	} else if exists {
		hasSetter = true
		setter = s
	}

	// Per ECMAScript 8.10.5 step 7.b/8.b: If 'get' or 'set' are not callable and not undefined, throw TypeError
	if hasGetter && getter.Type() != vm.TypeUndefined && !getter.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError("Getter must be a function")
	}
	if hasSetter && setter.Type() != vm.TypeUndefined && !setter.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError("Setter must be a function")
	}

	// Per ECMAScript 8.10.5: If accessor fields (get/set) present with data fields (value/writable), throw TypeError
	if (hasGetter || hasSetter) && (hasValue || writablePtr != nil) {
		return vm.Undefined, vmInstance.NewTypeError("Invalid property descriptor. Cannot both specify accessors and a value or writable attribute")
	}

	// A callable's "length", "name" and "prototype" are synthesized on demand
	// rather than stored, so defining over one would otherwise create a fresh
	// property with all-false attributes instead of merging with the real
	// descriptor - making a second defineProperty on it a rejection.
	if obj.IsCallable() {
		vm.MaterializeIntrinsicOwnProperties(vmInstance, obj)
	}

	// ValidateAndApplyPropertyDescriptor step 5 (ES2025 10.1.6.3): if every
	// field of the descriptor is absent, redefining an *existing* property is a
	// no-op that always succeeds - even a non-configurable one, which the
	// rejection paths below would otherwise refuse.
	if !hasValue && !hasWritable && !hasGetter && !hasSetter && enumerablePtr == nil && configurablePtr == nil {
		if target := definePropertyTarget(vmInstance, obj); target != nil {
			exists := false
			if keyIsSymbol {
				_, _, _, _, exists = target.GetOwnDescriptorByKey(vm.NewSymbolKey(propSym))
			} else {
				_, _, _, _, exists = target.GetOwnDescriptor(propName)
			}
			if exists {
				return obj, nil
			}
		}
	}

	// Functions, RegExps, Maps and Sets keep their own properties in a side
	// table; a brand-new property needs that table to be extensible, the same
	// check the TypeObject path below makes on the object itself.
	if props := vm.OwnPropertiesTable(obj); props != nil && !props.IsExtensible() {
		exists := false
		if keyIsSymbol {
			_, _, _, _, exists = props.GetOwnDescriptorByKey(vm.NewSymbolKey(propSym))
		} else {
			_, _, _, _, exists = props.GetOwnDescriptor(propName)
		}
		if !exists {
			keyStr := propName
			if keyIsSymbol {
				keyStr = propSym.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError("Cannot define property " + keyStr + ", object is not extensible")
		}
	}

	// Handle BoundFunction first (before AsPlainObject which would panic)
	if obj.Type() == vm.TypeBoundFunction {
		bf := obj.AsBoundFunction()
		if bf != nil {
			if bf.Properties == nil {
				bf.Properties = vm.EnsureOwnPropertiesTable(obj)
			}
			var defined bool
			if hasGetter || hasSetter {
				if keyIsSymbol {
					defined = bf.Properties.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					defined = bf.Properties.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					defined = bf.Properties.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					defined = bf.Properties.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
			if !defined {
				return vm.Undefined, definePropertyRejected(vmInstance, propName, propSym, keyIsSymbol)
			}
		}
		return obj, nil
	}

	// Arguments objects: implement ES 10.4.4.7 for numeric-index keys (the
	// mapped-argument write-through/severance semantics). "length"/"callee"
	// and symbol keys aren't covered yet - falls through to the no-op below,
	// same pre-existing behavior as before this case existed.
	if obj.Type() == vm.TypeArguments && !keyIsSymbol {
		if _, isIndex := vm.ParseArgumentsIndex(propName); isIndex {
			argsObj := obj.AsArguments()
			if err := vmInstance.ArgumentsDefineOwnProperty(argsObj, propName, hasValue, value, writablePtr, enumerablePtr, configurablePtr, hasGetter, getter, hasSetter, setter); err != nil {
				return vm.Undefined, err
			}
			return obj, nil
		}
	}

	// Array objects: Object.defineProperty(arr, "length", desc) - ES
	// 10.4.2.1 step 3 -> ArraySetLength (10.4.2.4). Only the "writable"
	// attribute is implemented (see ArrayObject.IsLengthWritable/
	// SetLengthWritable); a descriptor that also tries to change the
	// length *value* to something other than the array's current length
	// would need ArraySetLength's truncate-from-the-top semantics, which
	// aren't implemented - such a call is a no-op (matching this
	// function's pre-existing behavior for "length" before this block
	// existed) rather than applying any requested writable attribute
	// alongside it. get/set on "length" is always rejected: it's
	// inherently a data property.
	if obj.Type() == vm.TypeArray && !keyIsSymbol && propName == "length" {
		arr := obj.AsArray()
		if arr != nil {
			if hasGetter || hasSetter {
				return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property: length")
			}
			if enumerablePtr != nil && *enumerablePtr {
				return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property: length")
			}
			if configurablePtr != nil && *configurablePtr {
				return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property: length")
			}
			if !hasValue || uint32(int64(value.ToFloat())) == uint32(arr.Length()) {
				if writablePtr != nil {
					if !arr.IsLengthWritable() && *writablePtr {
						return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property: length")
					}
					arr.SetLengthWritable(*writablePtr)
				}
			}
			// else: a genuine value change - left untouched, same as this
			// function's pre-existing no-op behavior for "length" before
			// this block existed (ArraySetLength's truncation semantics
			// aren't implemented - see this block's doc comment).
			return obj, nil
		}
	}

	// Array objects: implement ES 10.4.2.1 Array exotic [[DefineOwnProperty]]
	// for everything except "length" (ArraySetLength's truncate-from-the-top
	// semantics aren't implemented - see ArrayDefineOwnProperty's doc
	// comment - so a "length" redefinition falls through to the no-op
	// below, same pre-existing behavior as before this branch existed).
	if obj.Type() == vm.TypeArray && !keyIsSymbol && propName != "length" {
		arr := obj.AsArray()
		if arr != nil {
			if err := vmInstance.ArrayDefineOwnProperty(arr, propName, hasValue, value, writablePtr, enumerablePtr, configurablePtr, hasGetter, getter, hasSetter, setter); err != nil {
				return vm.Undefined, err
			}
			return obj, nil
		}
	}

	// Array objects, symbol key: ES 10.4.2.1 Array exotic [[DefineOwnProperty]]
	// defers to OrdinaryDefineOwnProperty for any key that "length" doesn't
	// intercept - true for every symbol key, since a symbol can never equal
	// the string "length". This used to have no branch at all: a symbol key
	// on an array fell through every propName-gated check above (all of them
	// meaningless for a symbol, since propName is "" here) and reached the
	// `obj.Type() == vm.TypeObject` block below, which a TypeArray value
	// never satisfies - so Object.defineProperty(arr, sym, {...}) silently
	// did nothing: it neither stored anything nor threw, for both data and
	// accessor descriptors alike.
	if obj.Type() == vm.TypeArray && keyIsSymbol {
		arr := obj.AsArray()
		if arr != nil && propSym.AsSymbolObject() != nil {
			if err := vmInstance.ArrayDefineOwnSymbolProperty(arr, propSym.AsSymbolObject(), hasValue, value, writablePtr, enumerablePtr, configurablePtr, hasGetter, getter, hasSetter, setter); err != nil {
				return vm.Undefined, err
			}
			return obj, nil
		}
	}

	// Define the property with attributes (on plain objects only for now)
	if obj.Type() == vm.TypeObject {
		if obj.Type() == vm.TypeObject {
			plainObj := obj.AsPlainObject()
			// Check if property already exists and get existing attributes
			var exists bool
			var w0, e0, c0 bool
			var isAccessor0 bool
			if keyIsSymbol {
				if g, s, e, c, ok := plainObj.GetOwnAccessorByKey(vm.NewSymbolKey(propSym)); ok {
					isAccessor0, e0, c0, exists = true, e, c, true
					_ = g
					_ = s
				} else {
					_, w0, e0, c0, exists = plainObj.GetOwnDescriptorByKey(vm.NewSymbolKey(propSym))
				}
			} else {
				if g, s, e, c, ok := plainObj.GetOwnAccessor(propName); ok {
					isAccessor0, e0, c0, exists = true, e, c, true
					_ = g
					_ = s
				} else {
					_, w0, e0, c0, exists = plainObj.GetOwnDescriptor(propName)
				}
			}

			// Per ECMAScript spec:
			// - When creating a new property, missing attributes default to false
			// - When updating an existing property, missing attributes are preserved
			if exists {
				// Preserve existing attributes for missing descriptor fields
				if !(hasGetter || hasSetter) && writablePtr == nil {
					writablePtr = &w0
				}
				if enumerablePtr == nil {
					enumerablePtr = &e0
				}
				if configurablePtr == nil {
					configurablePtr = &c0
				}
				// Preserve existing value when descriptor doesn't specify a value
				// and we're not converting to an accessor property
				if !hasValue && !isAccessor0 && !(hasGetter || hasSetter) {
					if keyIsSymbol {
						if existingVal, ok := plainObj.GetOwnByKey(vm.NewSymbolKey(propSym)); ok {
							value = existingVal
						}
					} else {
						if existingVal, ok := plainObj.GetOwn(propName); ok {
							value = existingVal
						}
					}
				}
			} else {
				// New property: check extensibility first
				if !plainObj.IsExtensible() {
					keyStr := propName
					if keyIsSymbol {
						keyStr = propSym.ToString()
					}
					return vm.Undefined, vmInstance.NewTypeError("Cannot define property " + keyStr + ", object is not extensible")
				}
				// New property: default missing attributes to false
				if !(hasGetter || hasSetter) {
					if writablePtr == nil {
						b := false
						writablePtr = &b
					}
				}
				if enumerablePtr == nil {
					b := false
					enumerablePtr = &b
				}
				if configurablePtr == nil {
					b := false
					configurablePtr = &b
				}
			}

			if exists && !c0 {
				// Non-configurable property validation - throw TypeError per DefinePropertyOrThrow
				if configurablePtr != nil && *configurablePtr != c0 {
					return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property: " + propName)
				}
				if enumerablePtr != nil && *enumerablePtr != e0 {
					return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property: " + propName)
				}
				// If data non-writable cannot make writable true
				if !isAccessor0 && !w0 && writablePtr != nil && *writablePtr {
					return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property: " + propName)
				}
				// Disallow converting kind when not configurable (step 7c)
				// A generic descriptor (no value/writable/get/set) does NOT trigger kind conversion
				if isAccessor0 && (hasValue || hasWritable) {
					return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property: " + propName)
				}
				if !isAccessor0 && (hasGetter || hasSetter) {
					return vm.Undefined, vmInstance.NewTypeError("Cannot redefine property: " + propName)
				}
			}
			// A generic descriptor (no value/writable/get/set - e.g. just
			// {enumerable: true}) must not change an existing property's
			// kind (ValidateAndApplyPropertyDescriptor, ES2025 10.1.6.3): if
			// the property is already an accessor, route it through the
			// accessor path (which leaves an unspecified getter/setter
			// untouched) rather than the data path, which would otherwise
			// stomp the accessor with a data property whose value defaults
			// to undefined.
			useAccessorPath := hasGetter || hasSetter || (exists && isAccessor0 && !hasValue && !hasWritable)
			var defined bool
			if useAccessorPath {
				// Accessor path
				if keyIsSymbol {
					defined = plainObj.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					defined = plainObj.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					defined = plainObj.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					defined = plainObj.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
			if !defined {
				return vm.Undefined, definePropertyRejected(vmInstance, propName, propSym, keyIsSymbol)
			}
		}
	} else if obj.Type() == vm.TypeDictObject {
		// DictObject has no attributes; set value only for string keys; symbols unsupported
		if !keyIsSymbol {
			obj.AsDictObject().SetOwn(propName, value)
		}
	} else if obj.Type() == vm.TypeNativeFunctionWithProps {
		// NativeFunctionWithProps (like Function.prototype) stores properties in Properties
		nfp := obj.AsNativeFunctionWithProps()
		if nfp != nil && nfp.Properties != nil {
			var defined bool
			if hasGetter || hasSetter {
				if keyIsSymbol {
					defined = nfp.Properties.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					defined = nfp.Properties.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					defined = nfp.Properties.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					defined = nfp.Properties.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
			if !defined {
				return vm.Undefined, definePropertyRejected(vmInstance, propName, propSym, keyIsSymbol)
			}
		}
	} else if obj.Type() == vm.TypeNativeFunction {
		// Plain native functions (e.g. Array.prototype.push) store additional
		// properties in Properties exactly like TypeNativeFunctionWithProps
		// above - the isObjectLike gate near the top of this function used to
		// exclude TypeNativeFunction entirely, so Object.defineProperty on
		// one of these threw "called on non-object" even though a
		// bracket-notation assignment on the exact same value already wrote
		// into this same table fine (pkg/vm/properties_table.go's
		// ownPropertiesSlot has always listed TypeNativeFunction alongside
		// the other four callable kinds).
		nf := obj.AsNativeFunction()
		if nf != nil {
			if nf.Properties == nil {
				nf.Properties = vm.EnsureOwnPropertiesTable(obj)
			}
			var defined bool
			if hasGetter || hasSetter {
				if keyIsSymbol {
					defined = nf.Properties.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					defined = nf.Properties.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					defined = nf.Properties.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					defined = nf.Properties.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
			if !defined {
				return vm.Undefined, definePropertyRejected(vmInstance, propName, propSym, keyIsSymbol)
			}
		}
	} else if obj.Type() == vm.TypeFunction {
		// Functions store additional properties in Properties field
		fn := obj.AsFunction()
		if fn != nil {
			if fn.Properties == nil {
				fn.Properties = vm.EnsureOwnPropertiesTable(obj)
			}
			var defined bool
			if hasGetter || hasSetter {
				if keyIsSymbol {
					defined = fn.Properties.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					defined = fn.Properties.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					defined = fn.Properties.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					defined = fn.Properties.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
			if !defined {
				return vm.Undefined, definePropertyRejected(vmInstance, propName, propSym, keyIsSymbol)
			}
		}
	} else if obj.Type() == vm.TypeClosure {
		// Closures store additional properties in their own Properties field
		// (not the shared FunctionObject's) so distinct closures over the same
		// function body don't leak properties into each other.
		cl := obj.AsClosure()
		if cl != nil {
			if cl.Properties == nil {
				cl.Properties = vm.EnsureOwnPropertiesTable(obj)
			}
			var defined bool
			if hasGetter || hasSetter {
				if keyIsSymbol {
					defined = cl.Properties.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					defined = cl.Properties.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					defined = cl.Properties.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					defined = cl.Properties.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
			if !defined {
				return vm.Undefined, definePropertyRejected(vmInstance, propName, propSym, keyIsSymbol)
			}
		}
	} else if obj.Type() == vm.TypeRegExp || obj.Type() == vm.TypeMap || obj.Type() == vm.TypeSet || obj.Type() == vm.TypePromise {
		// These exotic kinds keep their ordinary own properties in the same
		// lazily-allocated side table Function/Closure use above
		// (OwnPropertiesTable/EnsureOwnPropertiesTable, pkg/vm/
		// properties_table.go, shared by ownPropertiesSlot's switch) - this
		// branch was missing entirely, so a fresh RegExp/Map/Set/Promise
		// (no table yet - the common case, since nothing else forces one
		// into existence first) made Object.defineProperty/
		// Reflect.defineProperty silently do nothing while still returning
		// the object as if it had succeeded.
		//
		// RegExp's "lastIndex" is excluded: it is a real Go field on
		// RegExpObject (see reflectDeleteProperty's TypeRegExp case -
		// {writable: true, configurable: false}), not a side-table entry -
		// a table write here would create a second, disconnected
		// "lastIndex" that shadows nothing real. Redefining lastIndex
		// through defineProperty remains a pre-existing, untouched gap
		// (still the same no-op it already was, not a new regression).
		if obj.Type() == vm.TypeRegExp && !keyIsSymbol && propName == "lastIndex" {
			// no-op - see comment above.
		} else if props := vm.EnsureOwnPropertiesTable(obj); props != nil {
			isAccessor0 := false
			if keyIsSymbol {
				if _, _, _, _, ok := props.GetOwnAccessorByKey(vm.NewSymbolKey(propSym)); ok {
					isAccessor0 = true
				}
			} else if _, _, _, _, ok := props.GetOwnAccessor(propName); ok {
				isAccessor0 = true
			}
			// Preserve the existing value when the descriptor doesn't
			// specify one and isn't converting to/from an accessor -
			// DefineOwnProperty otherwise overwrites an existing data
			// property's value with the zero Value{} passed here (mirrors
			// the TypeObject branch above).
			if !hasValue && !isAccessor0 && !(hasGetter || hasSetter) {
				if keyIsSymbol {
					if existingVal, ok := props.GetOwnByKey(vm.NewSymbolKey(propSym)); ok {
						value = existingVal
					}
				} else if existingVal, ok := props.GetOwn(propName); ok {
					value = existingVal
				}
			}
			var defined bool
			if hasGetter || hasSetter {
				if keyIsSymbol {
					defined = props.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					defined = props.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					defined = props.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					defined = props.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
			if !defined {
				return vm.Undefined, definePropertyRejected(vmInstance, propName, propSym, keyIsSymbol)
			}
		}
	}

	return obj, nil
}

func objectGetOwnPropertyDescriptorWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) < 1 {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}

	obj := args[0]

	// Step 1: ToObject(O) - throw TypeError for undefined/null
	if obj.Type() == vm.TypeUndefined || obj.Type() == vm.TypeNull {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}

	// Step 2: ToPropertyKey(P) - handle object keys via ToPrimitive
	// If P is not provided, it defaults to undefined which becomes "undefined" string
	var keyIsSymbol bool
	var propName string
	var propSym vm.Value
	var keyArg vm.Value
	if len(args) >= 2 {
		keyArg = args[1]
	} else {
		keyArg = vm.Undefined
	}
	if keyArg.Type() == vm.TypeSymbol {
		keyIsSymbol = true
		propSym = keyArg
	} else if keyArg.IsObject() || keyArg.IsCallable() {
		// ToPropertyKey: call ToPrimitive with "string" hint first
		primKey := vmInstance.ToPrimitive(keyArg, "string")
		// Check if ToPrimitive threw an exception
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert object to primitive value")
		}
		if primKey.Type() == vm.TypeSymbol {
			keyIsSymbol = true
			propSym = primKey
		} else {
			propName = primKey.ToString()
		}
	} else {
		propName = keyArg.ToString()
	}

	// Handle string primitives: treat as String exotic object
	if obj.Type() == vm.TypeString {
		str := vm.AsString(obj)
		if !keyIsSymbol {
			if propName == "length" {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", vm.NumberValue(float64(len([]rune(str)))))
				descriptor.SetOwn("writable", vm.BooleanValue(false))
				descriptor.SetOwn("enumerable", vm.BooleanValue(false))
				descriptor.SetOwn("configurable", vm.BooleanValue(false))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
			runes := []rune(str)
			if index, err := strconv.Atoi(propName); err == nil && index >= 0 && index < len(runes) {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", vm.NewString(string(runes[index])))
				descriptor.SetOwn("writable", vm.BooleanValue(false))
				descriptor.SetOwn("enumerable", vm.BooleanValue(true))
				descriptor.SetOwn("configurable", vm.BooleanValue(false))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
		}
		return vm.Undefined, nil
	}

	// Handle Proxy objects
	if obj.Type() == vm.TypeProxy {
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot get property descriptor on revoked Proxy")
		}

		// Check for getOwnPropertyDescriptor trap. GetMethod(handler,
		// "getOwnPropertyDescriptor") per spec: an inherited trap counts,
		// not just an own one - vmInstance.ProxyGetTrap (not a bare
		// proxy.Handler().AsPlainObject().GetOwn("getOwnPropertyDescriptor"))
		// for the same reason documented on its pkg/vm definition.
		if getTrap, ok := vmInstance.ProxyGetTrap(proxy.Handler(), "getOwnPropertyDescriptor"); ok && !getTrap.IsUndefined() && getTrap.Type() != vm.TypeNull {
			// Validate trap is callable
			if !getTrap.IsFunction() {
				return vm.Undefined, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap is not a function")
			}

			// Convert property key to appropriate value
			var propKey vm.Value
			if keyIsSymbol {
				propKey = propSym
			} else {
				propKey = vm.NewString(propName)
			}

			// Call handler.getOwnPropertyDescriptor(target, property)
			trapArgs := []vm.Value{proxy.Target(), propKey}
			result, err := vmInstance.Call(getTrap, proxy.Handler(), trapArgs)
			if err != nil {
				return vm.Undefined, err
			}

			// Step 9: Result must be undefined or an object
			if result.Type() != vm.TypeUndefined && !result.IsObject() {
				return vm.Undefined, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap result must be an object or undefined")
			}

			// ECMAScript 10.5.5 steps 11-22: Validate invariants
			target := proxy.Target()

			// Step 11: Get the target's own property descriptor
			var targetDescFound bool
			var targetConfigurable bool
			if target.Type() == vm.TypeObject {
				targetObj := target.AsPlainObject()
				_, _, _, targetConfigurable, targetDescFound = targetObj.GetOwnDescriptor(propName)
			} else if target.Type() == vm.TypeArray {
				arrObj := target.AsArray()
				if propName == "length" {
					targetDescFound = true
					targetConfigurable = false
				} else if index, parseErr := strconv.Atoi(propName); parseErr == nil && index >= 0 && arrObj.HasOwnIndexProperty(propName, index) {
					targetDescFound = true
					targetConfigurable = !arrObj.IsFrozen()
				} else if _, desc, ok := arrObj.GetOwnPropertyDescriptor(propName); ok {
					_ = desc
					targetDescFound = true
					targetConfigurable = desc.Configurable
				}
			}

			// Step 12: Check target extensibility
			targetExtensible := true
			if target.Type() == vm.TypeObject {
				targetObj := target.AsPlainObject()
				targetExtensible = targetObj.IsExtensible()
			} else if target.Type() == vm.TypeArray {
				targetExtensible = target.AsArray().IsExtensible()
			}

			if result.Type() == vm.TypeUndefined {
				// Step 14: If trapResult is undefined
				if targetDescFound {
					// Step 14a: If targetDesc exists
					if !targetConfigurable {
						// Step 14a.i: Can't report non-configurable property as non-existent
						return vm.Undefined, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap returned undefined for property '" + propName + "' which is non-configurable in the proxy target")
					}
					// Step 14b: If target is not extensible, can't report existing property as non-existent
					if !targetExtensible {
						return vm.Undefined, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap returned undefined for property '" + propName + "' which exists in the non-extensible proxy target")
					}
				}
				return vm.Undefined, nil
			}

			// Step 15-22: Result is a descriptor object, validate against target
			resultObj := result.AsPlainObject()
			if resultObj == nil {
				return result, nil
			}

			// Step 20: IsCompatiblePropertyDescriptor - non-extensible target can't have new properties reported
			if !targetExtensible && !targetDescFound {
				return vm.Undefined, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap returned descriptor for property '" + propName + "' on a non-extensible proxy target that does not have this property")
			}

			// Check if result descriptor says non-configurable
			resultConfigurable := true
			if confVal, hasConf := resultObj.GetOwn("configurable"); hasConf {
				resultConfigurable = !confVal.IsFalsey()
			}

			// Step 22: Invariant checks for non-configurable result
			if !resultConfigurable {
				if !targetDescFound {
					// Step 22a: Can't report non-configurable for property that doesn't exist on target
					return vm.Undefined, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap reported non-configurable for property '" + propName + "' which does not exist on the proxy target")
				}
				if targetConfigurable {
					// Step 22b: Can't report non-configurable when target property is configurable
					return vm.Undefined, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap reported non-configurable for property '" + propName + "' which is configurable in the proxy target")
				}
				// Step 22 additional: If result is non-configurable+non-writable, target must also be non-writable
				if writableVal, hasWritable := resultObj.GetOwn("writable"); hasWritable && writableVal.IsFalsey() {
					if target.Type() == vm.TypeObject {
						targetObj := target.AsPlainObject()
						_, targetWritable, _, _, found := targetObj.GetOwnDescriptor(propName)
						if found && targetWritable {
							return vm.Undefined, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap reported non-configurable and non-writable for property '" + propName + "' which is writable in the proxy target")
						}
					}
				}
			}

			return result, nil
		}

		// No trap, delegate to target
		return objectGetOwnPropertyDescriptorWithVM(vmInstance, []vm.Value{proxy.Target(), args[1]})
	}

	// First argument must be object-like (including functions)
	isObjectLike := obj.IsObject() || obj.Type() == vm.TypeArray ||
		obj.Type() == vm.TypeFunction || obj.Type() == vm.TypeClosure ||
		obj.Type() == vm.TypeNativeFunction || obj.Type() == vm.TypeNativeFunctionWithProps ||
		obj.Type() == vm.TypeAsyncNativeFunction || obj.Type() == vm.TypeBoundFunction
	if !isObjectLike {
		return vm.Undefined, nil
	}

	// Check if the property exists
	var value vm.Value

	// Handle function types - check Properties for custom props set via DefineMethod
	switch obj.Type() {
	case vm.TypeFunction:
		fn := obj.AsFunction()
		// Check custom properties first (e.g., static methods on constructor)
		if fn.Properties != nil {
			// Check accessor properties first
			if g, s, e, c, ok := func() (vm.Value, vm.Value, bool, bool, bool) {
				if keyIsSymbol {
					return fn.Properties.GetOwnAccessorByKey(vm.NewSymbolKey(propSym))
				}
				return fn.Properties.GetOwnAccessor(propName)
			}(); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("get", g)
				descriptor.SetOwn("set", s)
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
			// Then check data properties
			if v, w, e, c, ok := func() (vm.Value, bool, bool, bool, bool) {
				if keyIsSymbol {
					return fn.Properties.GetOwnDescriptorByKey(vm.NewSymbolKey(propSym))
				}
				return fn.Properties.GetOwnDescriptor(propName)
			}(); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", v)
				descriptor.SetOwn("writable", vm.BooleanValue(w))
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
		}
		// Fall through to check intrinsic properties below
	case vm.TypeClosure:
		closure := obj.AsClosure()
		// Check closure's own Properties first (where OpDefineMethod stores static methods)
		if closure.Properties != nil {
			// Check accessor properties first
			if g, s, e, c, ok := func() (vm.Value, vm.Value, bool, bool, bool) {
				if keyIsSymbol {
					return closure.Properties.GetOwnAccessorByKey(vm.NewSymbolKey(propSym))
				}
				return closure.Properties.GetOwnAccessor(propName)
			}(); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("get", g)
				descriptor.SetOwn("set", s)
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
			// Then check data properties
			if v, w, e, c, ok := func() (vm.Value, bool, bool, bool, bool) {
				if keyIsSymbol {
					return closure.Properties.GetOwnDescriptorByKey(vm.NewSymbolKey(propSym))
				}
				return closure.Properties.GetOwnDescriptor(propName)
			}(); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", v)
				descriptor.SetOwn("writable", vm.BooleanValue(w))
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
		}
		// Also check fn.Properties as fallback
		if closure.Fn.Properties != nil {
			// Check accessor properties first
			if g, s, e, c, ok := func() (vm.Value, vm.Value, bool, bool, bool) {
				if keyIsSymbol {
					return closure.Fn.Properties.GetOwnAccessorByKey(vm.NewSymbolKey(propSym))
				}
				return closure.Fn.Properties.GetOwnAccessor(propName)
			}(); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("get", g)
				descriptor.SetOwn("set", s)
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
			// Then check data properties
			if v, w, e, c, ok := func() (vm.Value, bool, bool, bool, bool) {
				if keyIsSymbol {
					return closure.Fn.Properties.GetOwnDescriptorByKey(vm.NewSymbolKey(propSym))
				}
				return closure.Fn.Properties.GetOwnDescriptor(propName)
			}(); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", v)
				descriptor.SetOwn("writable", vm.BooleanValue(w))
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
		}
		// Fall through to check intrinsic properties below
	case vm.TypeNativeFunctionWithProps:
		nfp := obj.AsNativeFunctionWithProps()
		// Check custom properties first
		if nfp.Properties != nil {
			// Check accessor properties first
			if g, s, e, c, ok := func() (vm.Value, vm.Value, bool, bool, bool) {
				if keyIsSymbol {
					return nfp.Properties.GetOwnAccessorByKey(vm.NewSymbolKey(propSym))
				}
				return nfp.Properties.GetOwnAccessor(propName)
			}(); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("get", g)
				descriptor.SetOwn("set", s)
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
			// Then check data properties
			if v, w, e, c, ok := func() (vm.Value, bool, bool, bool, bool) {
				if keyIsSymbol {
					return nfp.Properties.GetOwnDescriptorByKey(vm.NewSymbolKey(propSym))
				}
				return nfp.Properties.GetOwnDescriptor(propName)
			}(); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", v)
				descriptor.SetOwn("writable", vm.BooleanValue(w))
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
		}
		// Fall through to check intrinsic properties below
	}

	// Check arrays first before plainObj (arrays can also be AsPlainObject but their indices are stored separately)
	if obj.Type() == vm.TypeArray {
		arrObj := obj.AsArray()
		// Symbol-keyed own properties (arr[sym] = v, or an explicit
		// Object.defineProperty(arr, sym, {...})) live in ArrayObject's own
		// symbol-keyed storage (GetSymbolProp/SetSymbolProp/HasOwnSymbolProp
		// for a plain value; symbolPropertyDesc/symbolGetters/symbolSetters
		// for an explicit descriptor - see ArrayDefineOwnSymbolProperty,
		// pkg/vm/array_props.go) - completely separate from the
		// propName-based index/"length"/named-property checks below, all of
		// which are meaningless for a symbol key (propName is "" here). This
		// function used to have no symbol-key branch for TypeArray at all
		// (task_778749f8's fix here only handled the plain-value case, since
		// Object.defineProperty on a symbol key was itself still a no-op at
		// the time - see ArrayDefineOwnSymbolProperty's doc comment):
		//
		//   const arr = [1, 2]; const s = Symbol("d");
		//   Object.defineProperty(arr, s, {value: 7, writable: false, enumerable: false, configurable: false});
		//   Object.getOwnPropertyDescriptor(arr, s);
		//   // before: {value: 7, writable: true, enumerable: true, configurable: true} (hardcoded default)
		//   // Node:   {value: 7, writable: false, enumerable: false, configurable: false}
		if keyIsSymbol {
			if sym := propSym.AsSymbolObject(); sym != nil {
				if g, s, e, c, ok := arrObj.GetOwnSymbolAccessor(sym); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("get", g)
					descriptor.SetOwn("set", s)
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
				if v, desc, ok := arrObj.GetSymbolPropertyDescriptor(sym); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("value", v)
					descriptor.SetOwn("writable", vm.BooleanValue(desc.Writable))
					descriptor.SetOwn("enumerable", vm.BooleanValue(desc.Enumerable))
					descriptor.SetOwn("configurable", vm.BooleanValue(desc.Configurable))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
			}
			return vm.Undefined, nil
		}
		isFrozen := arrObj.IsFrozen()
		// An index (or named key) explicitly turned into an accessor via
		// Object.defineProperty takes priority over the plain-element read
		// below - see ArrayDefineOwnProperty's doc comment for why elements
		// and accessors are tracked separately.
		if g, s, e, c, ok := arrObj.GetOwnAccessor(propName); ok {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("get", g)
			descriptor.SetOwn("set", s)
			descriptor.SetOwn("enumerable", vm.BooleanValue(e))
			descriptor.SetOwn("configurable", vm.BooleanValue(c))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// For arrays, check if it's a valid index or 'length'
		if propName == "length" {
			value = vm.NumberValue(float64(arrObj.Length()))
			// length is non-enumerable, non-configurable; writable unless
			// frozen or explicitly made non-writable (Object.defineProperty
			// - see ArrayObject.IsLengthWritable).
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", value)
			descriptor.SetOwn("writable", vm.BooleanValue(arrObj.IsLengthWritable()))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(false))
			return vm.NewValueFromPlainObject(descriptor), nil
		} else if index, err := strconv.Atoi(propName); err == nil && index >= 0 && index < arrObj.Length() {
			// A hole (from `delete arr[i]`, a literal elision, or `new
			// Array(n)`) is not an own property at all - report undefined
			// rather than a data descriptor whose value happens to be
			// undefined (paserati#300).
			if !arrObj.HasOwnIndexProperty(propName, index) {
				return vm.Undefined, nil
			}
			value = arrObj.Get(index)
			// arrObj.Get only looks at the dense .elements slice; an index
			// beyond maxDenseArrayDefineIndex/maxDenseArraySetIndex is
			// tracked as a named property instead (see ArrayDefineOwnProperty),
			// so fall back the same way the bracket-read opcodes already do
			// (paserati#176) rather than reporting an empty value for it here.
			if !arrObj.HasIndex(index) {
				if v, ok := arrObj.GetOwn(propName); ok {
					value = v
				}
			}
			// The ES default (writable/configurable true unless frozen,
			// enumerable always true) unless an earlier Object.defineProperty
			// call on this same index tracked a different combination in
			// propertyDesc instead (paserati#178) - the write side
			// (ArrayDefineOwnProperty) is the only thing that ever puts an
			// entry there for a plain (non-accessor) index. Object.freeze
			// only ever flips the array-wide `frozen` flag (SetFrozen), never
			// touching propertyDesc, so a tracked writable/configurable:true
			// from before the freeze must still be ANDed with !isFrozen here -
			// freeze can only take capabilities away, never hand back one an
			// explicit defineProperty granted.
			writable, enumerable, configurable := !isFrozen, true, !isFrozen
			if desc, ok := arrObj.GetIndexAttributesOverride(propName); ok {
				writable, enumerable, configurable = desc.Writable && !isFrozen, desc.Enumerable, desc.Configurable && !isFrozen
			}
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", value)
			descriptor.SetOwn("writable", vm.BooleanValue(writable))
			descriptor.SetOwn("enumerable", vm.BooleanValue(enumerable))
			descriptor.SetOwn("configurable", vm.BooleanValue(configurable))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// Check custom properties on the array (e.g., "raw" for template objects, "index"/"input" for regex matches)
		if v, desc, ok := arrObj.GetOwnPropertyDescriptor(propName); ok {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", v)
			descriptor.SetOwn("writable", vm.BooleanValue(desc.Writable))
			descriptor.SetOwn("enumerable", vm.BooleanValue(desc.Enumerable))
			descriptor.SetOwn("configurable", vm.BooleanValue(desc.Configurable))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// Fall through for non-index properties on arrays (methods, custom props)
	}

	// Check type before calling As* methods to avoid panic
	switch obj.Type() {
	case vm.TypeObject:
		plainObj := obj.AsPlainObject()
		// String exotic object: check [[PrimitiveValue]] for index properties
		if !keyIsSymbol {
			if primVal, hasPrim := plainObj.GetOwn("[[PrimitiveValue]]"); hasPrim && primVal.Type() == vm.TypeString {
				str := vm.AsString(primVal)
				runes := []rune(str)
				if propName == "length" {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("value", vm.NumberValue(float64(len(runes))))
					descriptor.SetOwn("writable", vm.BooleanValue(false))
					descriptor.SetOwn("enumerable", vm.BooleanValue(false))
					descriptor.SetOwn("configurable", vm.BooleanValue(false))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
				if index, err := strconv.Atoi(propName); err == nil && index >= 0 && index < len(runes) {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("value", vm.NewString(string(runes[index])))
					descriptor.SetOwn("writable", vm.BooleanValue(false))
					descriptor.SetOwn("enumerable", vm.BooleanValue(true))
					descriptor.SetOwn("configurable", vm.BooleanValue(false))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
			}
		}
		if g, s, e, c, ok := func() (vm.Value, vm.Value, bool, bool, bool) {
			if keyIsSymbol {
				return plainObj.GetOwnAccessorByKey(vm.NewSymbolKey(propSym))
			}
			return plainObj.GetOwnAccessor(propName)
		}(); ok {
			// Accessor descriptor - always include both get and set per spec
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("get", g)
			descriptor.SetOwn("set", s)
			descriptor.SetOwn("enumerable", vm.BooleanValue(e))
			descriptor.SetOwn("configurable", vm.BooleanValue(c))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		if v, w, e, c, ok := func() (vm.Value, bool, bool, bool, bool) {
			if keyIsSymbol {
				return plainObj.GetOwnDescriptorByKey(vm.NewSymbolKey(propSym))
			}
			return plainObj.GetOwnDescriptor(propName)
		}(); ok {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", v)
			descriptor.SetOwn("writable", vm.BooleanValue(w))
			descriptor.SetOwn("enumerable", vm.BooleanValue(e))
			descriptor.SetOwn("configurable", vm.BooleanValue(c))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// Fallback: synthesize accessor descriptor from __get__/__set__ conventions used by object literal emitter
		if !keyIsSymbol {
			getName := "__get__" + propName
			setName := "__set__" + propName
			var g vm.Value = vm.Undefined
			var s vm.Value = vm.Undefined
			if gv, ok := plainObj.GetOwn(getName); ok {
				g = gv
			}
			if sv, ok := plainObj.GetOwn(setName); ok {
				s = sv
			}
			if g.Type() != vm.TypeUndefined || s.Type() != vm.TypeUndefined {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("get", g)
				descriptor.SetOwn("set", s)
				// Object literal accessors default to enumerable:true, configurable:true
				descriptor.SetOwn("enumerable", vm.BooleanValue(true))
				descriptor.SetOwn("configurable", vm.BooleanValue(true))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
		}
	case vm.TypeDictObject:
		dictObj := obj.AsDictObject()
		if v, w, e, c, ok := dictObj.GetOwnDescriptor(propName); ok {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", v)
			descriptor.SetOwn("writable", vm.BooleanValue(w))
			descriptor.SetOwn("enumerable", vm.BooleanValue(e))
			descriptor.SetOwn("configurable", vm.BooleanValue(c))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
	case vm.TypeArguments:
		argsObj := obj.AsArguments()
		// Handle symbol-keyed properties (e.g., Symbol.iterator)
		if keyIsSymbol {
			if v, ok := argsObj.GetSymbolProp(propSym.AsSymbolObject()); ok {
				// Symbol.iterator is writable, non-enumerable, configurable per spec
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", v)
				descriptor.SetOwn("writable", vm.BooleanValue(true))
				descriptor.SetOwn("enumerable", vm.BooleanValue(false))
				descriptor.SetOwn("configurable", vm.BooleanValue(true))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
			// Symbol property not found
			return vm.Undefined, nil
		}
		// Handle numeric index or "length"
		if propName == "length" {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NumberValue(float64(argsObj.Length())))
			descriptor.SetOwn("writable", vm.BooleanValue(true))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// Check for numeric index - consult any Object.defineProperty
		// override (attributes and, once the mapping's been severed, the
		// stored value) instead of always synthesizing the
		// CreateMappedArgumentsObject default. See arguments_props.go.
		if _, isIndex := vm.ParseArgumentsIndex(propName); isIndex {
			own := argsObj.ArgumentsOwnProperty(propName)
			if !own.Exists {
				return vm.Undefined, nil
			}
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			if own.IsAccessor {
				if own.HasGetter {
					descriptor.SetOwn("get", own.Getter)
				} else {
					descriptor.SetOwn("get", vm.Undefined)
				}
				if own.HasSetter {
					descriptor.SetOwn("set", own.Setter)
				} else {
					descriptor.SetOwn("set", vm.Undefined)
				}
			} else {
				descriptor.SetOwn("value", own.Value)
				descriptor.SetOwn("writable", vm.BooleanValue(own.Writable))
			}
			descriptor.SetOwn("enumerable", vm.BooleanValue(own.Enumerable))
			descriptor.SetOwn("configurable", vm.BooleanValue(own.Configurable))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// Handle callee property
		if propName == "callee" {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			if argsObj.IsStrict() {
				// In strict mode: accessor descriptor with %ThrowTypeError% intrinsic as get/set
				// Per ECMAScript spec, the same %ThrowTypeError% function is used for both
				descriptor.SetOwn("get", vmInstance.ThrowTypeErrorFunc)
				descriptor.SetOwn("set", vmInstance.ThrowTypeErrorFunc)
				descriptor.SetOwn("enumerable", vm.BooleanValue(false))
				descriptor.SetOwn("configurable", vm.BooleanValue(false))
			} else {
				// In non-strict mode: data descriptor with callee value
				descriptor.SetOwn("value", argsObj.Callee())
				descriptor.SetOwn("writable", vm.BooleanValue(true))
				descriptor.SetOwn("enumerable", vm.BooleanValue(false))
				descriptor.SetOwn("configurable", vm.BooleanValue(true))
			}
			return vm.NewValueFromPlainObject(descriptor), nil
		}
	}

	// Handle function intrinsic properties: name, length, prototype
	// Per ECMAScript spec, these are own data properties with specific attributes:
	// - name: {writable: false, enumerable: false, configurable: true}
	// - length: {writable: false, enumerable: false, configurable: true}
	// - prototype: {writable: true, enumerable: false, configurable: false} (for non-arrow functions)
	switch obj.Type() {
	case vm.TypeFunction:
		fn := obj.AsFunction()
		// Check for deleted intrinsic properties - if deleted, skip (return undefined)
		if propName == "name" && !fn.DeletedName {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NewString(fn.Name))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		if propName == "length" && !fn.DeletedLength {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NumberValue(float64(fn.Length)))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		if propName == "prototype" && !fn.IsArrowFunction {
			proto := fn.GetOrCreatePrototypeWithVM(vmInstance)
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", proto)
			descriptor.SetOwn("writable", vm.BooleanValue(true))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(false))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
	case vm.TypeClosure:
		closure := obj.AsClosure()
		fn := closure.Fn
		// Check for deleted intrinsic properties - if deleted, skip (return undefined)
		if propName == "name" && !fn.DeletedName {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NewString(fn.Name))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		if propName == "length" && !fn.DeletedLength {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NumberValue(float64(fn.Length)))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		if propName == "prototype" && !fn.IsArrowFunction {
			proto := closure.GetPrototypeWithVM(vmInstance)
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", proto)
			descriptor.SetOwn("writable", vm.BooleanValue(true))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(false))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
	case vm.TypeNativeFunction:
		nf := obj.AsNativeFunction()
		// A symbol key is never "name"/"length" (a symbol never equals a
		// string), so it skips straight to the side-table lookup - mirroring
		// the TypeBoundFunction block below (accessor first, then data).
		// Before this, TypeNativeFunction never checked nf.Properties at
		// all here, symbol or string key: Object.defineProperty on a plain
		// native function (e.g. Array.prototype.push) now writes into that
		// table (a separate, sibling fix), but this getter still answered
		// undefined for what it had just written.
		if keyIsSymbol {
			if nf.Properties != nil {
				symKey := vm.NewSymbolKey(propSym)
				if g, s, e, c, ok := nf.Properties.GetOwnAccessorByKey(symKey); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("get", g)
					descriptor.SetOwn("set", s)
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
				if v, w, e, c, ok := nf.Properties.GetOwnDescriptorByKey(symKey); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("value", v)
					descriptor.SetOwn("writable", vm.BooleanValue(w))
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
			}
			return vm.Undefined, nil
		}
		if propName == "name" && !nf.DeletedName {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NewString(nf.Name))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		if propName == "length" && !nf.DeletedLength {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NumberValue(float64(nf.Arity)))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// Any other own property (set via bracket-notation assignment or
		// Object.defineProperty on this same table) - accessor first, then
		// data, mirroring the symbol-key check above.
		if nf.Properties != nil {
			if g, s, e, c, ok := nf.Properties.GetOwnAccessor(propName); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("get", g)
				descriptor.SetOwn("set", s)
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
			if v, w, e, c, ok := nf.Properties.GetOwnDescriptor(propName); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", v)
				descriptor.SetOwn("writable", vm.BooleanValue(w))
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
		}
	case vm.TypeNativeFunctionWithProps:
		nfp := obj.AsNativeFunctionWithProps()
		if propName == "name" && !nfp.DeletedName {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NewString(nfp.Name))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		if propName == "length" && !nfp.DeletedLength {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NumberValue(float64(nfp.Arity)))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
	case vm.TypeBoundFunction:
		bf := obj.AsBoundFunction()
		if bf.Properties != nil {
			// A symbol key never names the synthesized "name"/"length"
			// intrinsics above, so it skips straight to the side-table
			// lookup - mirroring the TypeMap/TypeSet/TypePromise block
			// below (accessor first, then data), which this case never
			// had at all: it only ever looked up `propName`, a string, so
			// `Object.getOwnPropertyDescriptor(boundFn, sym)` answered
			// undefined even for a real own symbol property that
			// Reflect.has/`in` already found correctly.
			if keyIsSymbol {
				symKey := vm.NewSymbolKey(propSym)
				if g, s, e, c, ok := bf.Properties.GetOwnAccessorByKey(symKey); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("get", g)
					descriptor.SetOwn("set", s)
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
				if v, w, e, c, ok := bf.Properties.GetOwnDescriptorByKey(symKey); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("value", v)
					descriptor.SetOwn("writable", vm.BooleanValue(w))
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
				return vm.Undefined, nil
			}
			// Bound function name/length are real own properties in bf.Properties
			// They can be deleted (configurable:true) or redefined via Object.defineProperty
			if propName == "name" || propName == "length" {
				if val, ok := bf.Properties.GetOwn(propName); ok {
					_, w, e, c, _ := bf.Properties.GetOwnDescriptor(propName)
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("value", val)
					descriptor.SetOwn("writable", vm.BooleanValue(w))
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
				// Property was deleted — return undefined
				return vm.Undefined, nil
			}
			// Other properties
			if val, ok := bf.Properties.GetOwn(propName); ok {
				_, w, e, c, _ := bf.Properties.GetOwnDescriptor(propName)
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", val)
				descriptor.SetOwn("writable", vm.BooleanValue(w))
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
		}
	}

	// Handle RegExp intrinsic property: lastIndex
	// Per ECMAScript spec: {value: 0, writable: true, enumerable: false, configurable: false}
	if obj.Type() == vm.TypeRegExp {
		// A symbol key is never "lastIndex" (a symbol never equals a
		// string), so that intrinsic check stays string-only and this
		// skips straight to the side-table lookup - mirroring the
		// TypeMap/TypeSet/TypePromise block above (accessor first, then
		// data), which the "custom properties on the regex" check just
		// below never had at all: it only ever looked up `propName`, a
		// string, so `Object.getOwnPropertyDescriptor(regex, sym)`
		// answered undefined even for a real own symbol property that
		// Reflect.has/`in` already found correctly.
		if keyIsSymbol {
			regexObj := obj.AsRegExpObject()
			if regexObj != nil && regexObj.Properties != nil {
				symKey := vm.NewSymbolKey(propSym)
				if g, s, e, c, ok := regexObj.Properties.GetOwnAccessorByKey(symKey); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("get", g)
					descriptor.SetOwn("set", s)
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
				if v, w, e, c, ok := regexObj.Properties.GetOwnDescriptorByKey(symKey); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("value", v)
					descriptor.SetOwn("writable", vm.BooleanValue(w))
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
			}
			return vm.Undefined, nil
		}
		if propName == "lastIndex" {
			regexObj := obj.AsRegExpObject()
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.Number(float64(regexObj.GetLastIndex())))
			descriptor.SetOwn("writable", vm.BooleanValue(true))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(false))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// Check custom properties on the regex
		regexObj := obj.AsRegExpObject()
		if regexObj != nil && regexObj.Properties != nil {
			if v, w, e, c, ok := regexObj.Properties.GetOwnDescriptor(propName); ok {
				descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				descriptor.SetOwn("value", v)
				descriptor.SetOwn("writable", vm.BooleanValue(w))
				descriptor.SetOwn("enumerable", vm.BooleanValue(e))
				descriptor.SetOwn("configurable", vm.BooleanValue(c))
				return vm.NewValueFromPlainObject(descriptor), nil
			}
		}
	}

	// Map/Set/Promise have no intrinsic own properties of their own (unlike
	// RegExp's "lastIndex" above) - only whatever a program has defined on
	// the same side table via Object/Reflect.defineProperty or a plain
	// assignment (OwnPropertiesTable, pkg/vm/properties_table.go). This case
	// was missing entirely, so Object.getOwnPropertyDescriptor always
	// answered undefined for a custom property on one of these three kinds.
	if obj.Type() == vm.TypeMap || obj.Type() == vm.TypeSet || obj.Type() == vm.TypePromise {
		if props := vm.OwnPropertiesTable(obj); props != nil {
			if keyIsSymbol {
				symKey := vm.NewSymbolKey(propSym)
				if g, s, e, c, ok := props.GetOwnAccessorByKey(symKey); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("get", g)
					descriptor.SetOwn("set", s)
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
				if v, w, e, c, ok := props.GetOwnDescriptorByKey(symKey); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("value", v)
					descriptor.SetOwn("writable", vm.BooleanValue(w))
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
			} else {
				if g, s, e, c, ok := props.GetOwnAccessor(propName); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("get", g)
					descriptor.SetOwn("set", s)
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
				if v, w, e, c, ok := props.GetOwnDescriptor(propName); ok {
					descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					descriptor.SetOwn("value", v)
					descriptor.SetOwn("writable", vm.BooleanValue(w))
					descriptor.SetOwn("enumerable", vm.BooleanValue(e))
					descriptor.SetOwn("configurable", vm.BooleanValue(c))
					return vm.NewValueFromPlainObject(descriptor), nil
				}
			}
		}
	}

	return vm.Undefined, nil
}

// objectGetOwnPropertyDescriptorsWithVM implements Object.getOwnPropertyDescriptors(obj)
// Returns an object with all own property descriptors of obj
func objectGetOwnPropertyDescriptorsWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}

	obj := args[0]

	// Handle null/undefined
	if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
	}

	// Create result object
	result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Collect all own property keys (string keys first, then symbols)
	var stringKeys []string
	var symbolKeys []vm.Value

	switch obj.Type() {
	case vm.TypeObject:
		if obj.Type() == vm.TypeObject {
			po := obj.AsPlainObject()
			stringKeys = po.OwnPropertyNames()
			symbolKeys = po.OwnSymbolKeys()
		} else if obj.Type() == vm.TypeDictObject {
			d := obj.AsDictObject()
			stringKeys = d.OwnPropertyNames()
		}
	case vm.TypeArray:
		arr := obj.AsArray()
		// This used to loop `i := 0; i < arr.Length()` - arr.Length() is
		// the array's .length property, which a huge sparse index (one
		// defined past maxDenseArrayDefineIndex via Object.defineProperty,
		// see ArrayDefineOwnProperty) extends without growing `elements`
		// at all, making this a multi-billion-iteration hang for an array
		// that otherwise holds a handful of elements - the exact
		// paserati#176/#178 shape every other array-key-collecting
		// operation in this file already guards against by bounding the
		// per-index scan to DenseLength() and walking arraySparseIndices
		// separately (O(number of sparse entries), not O(index value)).
		// It also never collected any NAMED (non-index) key at all -
		// plain or accessor - so `arr.foo = "bar"` and an
		// Object.defineProperty(arr, "foo", {get, ...}) accessor were
		// both silently dropped, on top of the loop's own hang risk.
		for i := 0; i < arr.DenseLength(); i++ {
			key := strconv.Itoa(i)
			if !arr.HasOwnIndexProperty(key, i) {
				continue // hole - not an own property at all (paserati#300)
			}
			stringKeys = append(stringKeys, key)
		}
		// getOwnPropertyDescriptors wants every own key regardless of
		// enumerability, hence enumerableOnly=false for both helpers below
		// - matching Object.getOwnPropertyNames' own TypeArray case.
		for _, idx := range arraySparseIndices(arr, false) {
			stringKeys = append(stringKeys, strconv.Itoa(idx))
		}
		stringKeys = append(stringKeys, "length")
		stringKeys = append(stringKeys, arrayNamedKeys(arr, false)...)
		// This case never collected symbol keys at all, so
		// Object.getOwnPropertyDescriptors(arr) silently dropped a symbol
		// property entirely (whether a plain `arr[sym] = v` or one defined
		// via Object.defineProperty - see ArrayDefineOwnSymbolProperty,
		// pkg/vm/array_props.go) despite the single-key
		// Object.getOwnPropertyDescriptor already answering correctly for
		// the exact same property - same gap this switch's other
		// branches' doc comments describe fixing for their own kinds.
		symbolKeys = append(symbolKeys, arr.OwnSymbolKeys()...)
	case vm.TypeFunction:
		fn := obj.AsFunction()
		// Function intrinsics: length, name, prototype
		if !fn.DeletedLength {
			stringKeys = append(stringKeys, "length")
		}
		if !fn.DeletedName {
			stringKeys = append(stringKeys, "name")
		}
		if fn.Properties != nil {
			if _, ok := fn.Properties.GetOwn("prototype"); ok {
				stringKeys = append(stringKeys, "prototype")
			}
		}
		// Add custom properties
		if fn.Properties != nil {
			for _, k := range fn.Properties.OwnPropertyNames() {
				// Avoid duplicates
				if k != "length" && k != "name" && k != "prototype" {
					stringKeys = append(stringKeys, k)
				}
			}
		}
	case vm.TypeClosure:
		closure := obj.AsClosure()
		// Closure intrinsics: length, name
		if !closure.Fn.DeletedLength {
			stringKeys = append(stringKeys, "length")
		}
		if !closure.Fn.DeletedName {
			stringKeys = append(stringKeys, "name")
		}
		// Check prototype
		if closure.Properties != nil {
			if _, ok := closure.Properties.GetOwn("prototype"); ok {
				stringKeys = append(stringKeys, "prototype")
			}
		}
		// Add custom properties
		if closure.Properties != nil {
			for _, k := range closure.Properties.OwnPropertyNames() {
				if k != "length" && k != "name" && k != "prototype" {
					stringKeys = append(stringKeys, k)
				}
			}
		}
	case vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps:
		// Native functions have length and name
		stringKeys = append(stringKeys, "length", "name")
		if obj.Type() == vm.TypeNativeFunctionWithProps {
			nfp := obj.AsNativeFunctionWithProps()
			if nfp.Properties != nil {
				for _, k := range nfp.Properties.OwnPropertyNames() {
					if k != "length" && k != "name" {
						stringKeys = append(stringKeys, k)
					}
				}
				// Symbol keys were missing here too (e.g. a symbol property
				// defined on a native constructor like Boolean/Number) -
				// same gap as the TypeNativeFunction branch below, fixed
				// alongside it since it's the identical one-line fix on the
				// identical *PlainObject side table.
				symbolKeys = append(symbolKeys, nfp.Properties.OwnSymbolKeys()...)
			}
		} else {
			// TypeNativeFunction: a plain native method's own custom
			// properties - this branch never existed at all, so
			// Object.getOwnPropertyDescriptors(nf) only ever reported
			// "length"/"name", silently dropping anything just defined via
			// Object.defineProperty or bracket-notation assignment (the
			// single-key Object.getOwnPropertyDescriptor already answered
			// correctly for the exact same property).
			nf := obj.AsNativeFunction()
			if nf.Properties != nil {
				for _, k := range nf.Properties.OwnPropertyNames() {
					if k != "length" && k != "name" {
						stringKeys = append(stringKeys, k)
					}
				}
				symbolKeys = append(symbolKeys, nf.Properties.OwnSymbolKeys()...)
			}
		}
	case vm.TypeBoundFunction:
		// This case didn't exist at all before this fix, so
		// Object.getOwnPropertyDescriptors(boundFn) always returned {}
		// entirely - missing even "name"/"length", unlike every other
		// callable kind's branch in this same switch (which all
		// synthesize those two explicitly, since for THEM name/length
		// aren't real entries in the side table). A bound function is the
		// one callable kind where "name" ("bound " + original name) and
		// "length" (computed at bind time) genuinely ARE real own
		// properties set directly into bf.Properties (see
		// pkg/vm/property_helpers.go's "Bound functions: 'name'/'length'
		// is a real own property set at bind time" comments), so unlike
		// TypeFunction/TypeNativeFunction*/TypeClosure above, no special
		// synthesis is needed here - a plain OwnPropertyNames()/
		// OwnSymbolKeys() walk already includes them alongside any other
		// custom own property (Object.defineProperty, bracket-notation
		// assignment).
		bf := obj.AsBoundFunction()
		if bf.Properties != nil {
			stringKeys = append(stringKeys, bf.Properties.OwnPropertyNames()...)
			symbolKeys = append(symbolKeys, bf.Properties.OwnSymbolKeys()...)
		}
	case vm.TypeRegExp, vm.TypeMap, vm.TypeSet, vm.TypePromise:
		// These exotic kinds keep their ordinary own properties in the same
		// lazily-allocated side table (OwnPropertiesTable, pkg/vm/
		// properties_table.go) as Function/Closure/NativeFunctionWithProps
		// above - this case was missing entirely, so
		// Object.getOwnPropertyDescriptors always returned {} for one of
		// these four kinds even after a real own property had been defined
		// or assigned on it, despite the single-key
		// Object.getOwnPropertyDescriptor already answering correctly for
		// the exact same property.
		//
		// RegExp's "lastIndex" is its own case: like TypeFunction's
		// synthesized "length"/"name"/"prototype" intrinsics above, it is a
		// real own property that isn't stored in the side table at all (a
		// Go field on RegExpObject instead - see
		// objectGetOwnPropertyDescriptorWithVM's TypeRegExp handling, same
		// file), so it has to be added explicitly rather than falling out
		// of OwnPropertyNames().
		if obj.Type() == vm.TypeRegExp {
			stringKeys = append(stringKeys, "lastIndex")
		}
		if props := vm.OwnPropertiesTable(obj); props != nil {
			stringKeys = append(stringKeys, props.OwnPropertyNames()...)
			symbolKeys = append(symbolKeys, props.OwnSymbolKeys()...)
		}
	}

	// Get descriptor for each string key
	for _, key := range stringKeys {
		desc, err := objectGetOwnPropertyDescriptorWithVM(vmInstance, []vm.Value{obj, vm.NewString(key)})
		if err != nil {
			return vm.Undefined, err
		}
		if desc.Type() != vm.TypeUndefined {
			result.SetOwn(key, desc)
		}
	}

	// Get descriptor for each symbol key
	for _, symVal := range symbolKeys {
		desc, err := objectGetOwnPropertyDescriptorWithVM(vmInstance, []vm.Value{obj, symVal})
		if err != nil {
			return vm.Undefined, err
		}
		if desc.Type() != vm.TypeUndefined {
			// Set symbol property using DefineOwnPropertyByKey
			w, e, c := true, true, true
			result.DefineOwnPropertyByKey(vm.NewSymbolKey(symVal), desc, &w, &e, &c)
		}
	}

	return vm.NewValueFromPlainObject(result), nil
}

func objectIsExtensibleWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.BooleanValue(false), nil
	}

	obj := args[0]

	// Handle Proxy objects
	if obj.Type() == vm.TypeProxy {
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot check extensibility of revoked Proxy")
		}

		// Check for isExtensible trap. GetMethod(handler, "isExtensible")
		// per spec: an inherited trap counts, not just an own one -
		// vmInstance.ProxyGetTrap (not a bare
		// proxy.Handler().AsPlainObject().GetOwn("isExtensible")) for the
		// same reason documented on its pkg/vm definition.
		if extTrap, ok := vmInstance.ProxyGetTrap(proxy.Handler(), "isExtensible"); ok && !extTrap.IsUndefined() && extTrap.Type() != vm.TypeNull {
			// Validate trap is callable
			if !extTrap.IsFunction() {
				return vm.Undefined, vmInstance.NewTypeError("'isExtensible' on proxy: trap is not a function")
			}

			// Call handler.isExtensible(target)
			trapArgs := []vm.Value{proxy.Target()}
			result, err := vmInstance.Call(extTrap, proxy.Handler(), trapArgs)
			if err != nil {
				return vm.Undefined, err
			}

			// ECMAScript 10.5.3: trap result must match target's IsExtensible
			trapResult := result.IsTruthy()
			targetExtensible := true
			if proxy.Target().Type() == vm.TypeObject {
				if proxy.Target().Type() == vm.TypeObject {
					targetObj := proxy.Target().AsPlainObject()
					targetExtensible = targetObj.IsExtensible()
				}
			} else if proxy.Target().Type() == vm.TypeArray {
				targetExtensible = proxy.Target().AsArray().IsExtensible()
			}
			if trapResult != targetExtensible {
				return vm.Undefined, vmInstance.NewTypeError("'isExtensible' on proxy: trap result does not reflect extensibility of proxy target")
			}
			return vm.BooleanValue(trapResult), nil
		}

		// No trap, delegate to target
		return objectIsExtensibleWithVM(vmInstance, []vm.Value{proxy.Target()})
	}

	// Handle arrays specially
	if obj.Type() == vm.TypeArray {
		arr := obj.AsArray()
		return vm.BooleanValue(arr.IsExtensible()), nil
	}

	// Check if object is extensible
	if obj.Type() == vm.TypeObject {
		return vm.BooleanValue(obj.AsPlainObject().IsExtensible()), nil
	}

	// Functions, RegExps, Maps and Sets record extensibility on their side
	// table. No table means nothing ever made the value non-extensible.
	// Checked before the catch-all below, which reaches RegExps and Maps too.
	if props := vm.OwnPropertiesTable(obj); props != nil {
		return vm.BooleanValue(props.IsExtensible()), nil
	}

	if obj.IsObject() {
		// Other object types (DictObject, etc.) are extensible by default for now
		return vm.BooleanValue(true), nil
	}

	// Check for %ThrowTypeError% intrinsic - it's NOT extensible per ECMAScript spec
	if obj.Type() == vm.TypeNativeFunction && vmInstance != nil &&
		vmInstance.ThrowTypeErrorFunc.Type() == vm.TypeNativeFunction &&
		obj.AsNativeFunction() == vmInstance.ThrowTypeErrorFunc.AsNativeFunction() {
		return vm.BooleanValue(false), nil
	}

	// Functions, closures, and native functions are objects and extensible by default per ECMAScript
	if obj.Type() == vm.TypeFunction || obj.Type() == vm.TypeClosure ||
		obj.Type() == vm.TypeNativeFunction || obj.Type() == vm.TypeNativeFunctionWithProps ||
		obj.Type() == vm.TypeBoundFunction || obj.Type() == vm.TypeAsyncNativeFunction {
		return vm.BooleanValue(true), nil
	}

	return vm.BooleanValue(false), nil
}

func objectPreventExtensionsWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, vmInstance.NewTypeError("Object.preventExtensions requires an argument")
	}

	obj := args[0]

	// Handle Proxy objects
	if obj.Type() == vm.TypeProxy {
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot prevent extensions on revoked Proxy")
		}

		// Check for preventExtensions trap. GetMethod(handler,
		// "preventExtensions") per spec: an inherited trap counts, not
		// just an own one - vmInstance.ProxyGetTrap (not a bare
		// proxy.Handler().AsPlainObject().GetOwn("preventExtensions")) for
		// the same reason documented on its pkg/vm definition.
		if prevTrap, ok := vmInstance.ProxyGetTrap(proxy.Handler(), "preventExtensions"); ok && !prevTrap.IsUndefined() && prevTrap.Type() != vm.TypeNull {
			// Validate trap is callable
			if !prevTrap.IsFunction() {
				return vm.Undefined, vmInstance.NewTypeError("'preventExtensions' on proxy: trap is not a function")
			}

			// Call handler.preventExtensions(target)
			trapArgs := []vm.Value{proxy.Target()}
			result, err := vmInstance.Call(prevTrap, proxy.Handler(), trapArgs)
			if err != nil {
				return vm.Undefined, err
			}

			// If trap returns falsy, throw TypeError (Object.preventExtensions spec step 3)
			// Reflect.preventExtensions catches this and returns false
			if result.IsFalsey() {
				return vm.Undefined, vmInstance.NewTypeError("'preventExtensions' on proxy: trap returned falsish")
			}

			// ECMAScript 10.5.4: if trap returns true, target must be non-extensible
			targetExtensible := true
			if proxy.Target().Type() == vm.TypeObject {
				if proxy.Target().Type() == vm.TypeObject {
					targetObj := proxy.Target().AsPlainObject()
					targetExtensible = targetObj.IsExtensible()
				}
			} else if proxy.Target().Type() == vm.TypeArray {
				targetExtensible = proxy.Target().AsArray().IsExtensible()
			}
			if targetExtensible {
				return vm.Undefined, vmInstance.NewTypeError("'preventExtensions' on proxy: trap returned truish but the proxy target is extensible")
			}

			return obj, nil
		}

		// No trap, delegate to target
		return objectPreventExtensionsWithVM(vmInstance, []vm.Value{proxy.Target()})
	}

	// Per ECMAScript spec, primitives are returned unchanged.
	// Callables are objects even when Value.IsObject() says otherwise.
	if !obj.IsObject() && !obj.IsCallable() {
		return obj, nil
	}

	// Handle arrays specially
	if obj.Type() == vm.TypeArray {
		arr := obj.AsArray()
		arr.SetExtensible(false)
		return obj, nil
	}

	// Mark the object as non-extensible
	switch obj.Type() {
	case vm.TypeObject:
		obj.AsPlainObject().SetExtensible(false)
	case vm.TypeDictObject:
		obj.AsDictObject().SetExtensible(false)
	default:
		// Functions, RegExps, Maps and Sets record extensibility on their side
		// table, created here if the value doesn't have one yet.
		if props := vm.EnsureOwnPropertiesTable(obj); props != nil {
			props.SetExtensible(false)
		}
	}

	return obj, nil
}

func objectFreezeWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, nil
	}

	obj := args[0]

	// If not an object, return as-is (primitives are already immutable).
	// Callables are objects even when Value.IsObject() says otherwise.
	if !obj.IsObject() && !obj.IsCallable() {
		return obj, nil
	}

	// Handle arrays specially
	if obj.Type() == vm.TypeArray {
		arr := obj.AsArray()
		arr.SetExtensible(false)
		arr.SetFrozen(true)
		return obj, nil
	}

	// Freeze: make object non-extensible and all properties non-configurable and non-writable
	if obj.Type() == vm.TypeObject {
		plainObj := obj.AsPlainObject()
		plainObj.SetExtensible(false)
		// Use FreezeAllProperties to freeze ALL own properties (including non-enumerable and symbol)
		plainObj.FreezeAllProperties()
	} else if props := vm.EnsureOwnPropertiesTable(obj); props != nil {
		// Functions, RegExps, Maps and Sets are objects too: their own
		// properties - and their extensible flag - live in a side table, which
		// is what has to be frozen. The table is created if the value doesn't
		// have one yet, so freezing a property-less function still sticks, and
		// a callable's synthesized own properties are materialized into it
		// first so the freeze covers them too.
		vm.MaterializeIntrinsicOwnProperties(vmInstance, obj)
		props.SetExtensible(false)
		props.FreezeAllProperties()
	}

	return obj, nil
}

func objectSealWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, nil
	}

	obj := args[0]

	// If not an object, return as-is.
	// Callables are objects even when Value.IsObject() says otherwise.
	if !obj.IsObject() && !obj.IsCallable() {
		return obj, nil
	}

	// Handle arrays specially
	if obj.Type() == vm.TypeArray {
		arr := obj.AsArray()
		arr.SetExtensible(false)
		arr.SealProperties() // Seal named properties but leave elements writable
		return obj, nil
	}

	// Seal: make object non-extensible and all properties non-configurable (but leave writable as-is)
	if obj.Type() == vm.TypeObject {
		plainObj := obj.AsPlainObject()
		plainObj.SetExtensible(false)
		plainObj.SealAllProperties()
	} else if props := vm.EnsureOwnPropertiesTable(obj); props != nil {
		// See objectFreezeWithVM.
		vm.MaterializeIntrinsicOwnProperties(vmInstance, obj)
		props.SetExtensible(false)
		props.SealAllProperties()
	}

	return obj, nil
}

func objectIsFrozenWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.BooleanValue(true), nil
	}

	obj := args[0]

	// Primitives are frozen
	if !obj.IsObject() && !obj.IsCallable() {
		return vm.BooleanValue(true), nil
	}

	// Handle arrays specially
	if obj.Type() == vm.TypeArray {
		arr := obj.AsArray()
		return vm.BooleanValue(!arr.IsExtensible() && arr.IsFrozen()), nil
	}

	// Check if extensible - frozen objects must not be extensible
	if obj.Type() == vm.TypeObject {
		plainObj := obj.AsPlainObject()
		if plainObj.IsExtensible() {
			return vm.BooleanValue(false), nil
		}
		return vm.BooleanValue(plainObj.IsFrozenProperties()), nil
	}

	// Functions, RegExps, Maps and Sets keep their own properties - and their
	// extensible flag - in a side table. No table means nothing has ever made
	// the value non-extensible, so it isn't frozen.
	if props := vm.OwnPropertiesTable(obj); props != nil {
		return vm.BooleanValue(!props.IsExtensible() && props.IsFrozenProperties()), nil
	}

	return vm.BooleanValue(false), nil
}

func objectIsSealedWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.BooleanValue(true), nil
	}

	obj := args[0]

	// Primitives are sealed
	if !obj.IsObject() && !obj.IsCallable() {
		return vm.BooleanValue(true), nil
	}

	// Handle arrays
	if obj.Type() == vm.TypeArray {
		arr := obj.AsArray()
		return vm.BooleanValue(!arr.IsExtensible()), nil
	}

	// Check if extensible - sealed objects must not be extensible
	if obj.Type() == vm.TypeObject {
		plainObj := obj.AsPlainObject()
		if plainObj.IsExtensible() {
			return vm.BooleanValue(false), nil
		}
		return vm.BooleanValue(plainObj.IsSealedProperties()), nil
	}

	// Side-table kinds: see objectIsFrozenWithVM.
	if props := vm.OwnPropertiesTable(obj); props != nil {
		return vm.BooleanValue(!props.IsExtensible() && props.IsSealedProperties()), nil
	}

	return vm.BooleanValue(false), nil
}

// ObjectGetOwnPropertyDescriptorForHarness exposes a minimal descriptor getter for the test262 harness
func ObjectGetOwnPropertyDescriptorForHarness(obj vm.Value, name vm.Value) (vm.Value, error) {
	return objectGetOwnPropertyDescriptorWithVM(nil, []vm.Value{obj, name})
}

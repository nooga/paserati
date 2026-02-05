package builtins

import (
	"math"
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
		thisValue := vmInstance.GetThis()
		keyVal := args[0]
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
			// Check numeric indices
			if index, err := strconv.Atoi(propName); err == nil {
				return vm.BooleanValue(index >= 0 && index < arrObj.Length()), nil
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
			// prototype is only an own property for non-arrow functions that are NOT methods
			// Arrow functions and methods don't have prototype
			// We check if it exists in Properties (created lazily or explicitly)
			// or if the function is a class constructor (always has prototype)
			if propName == "prototype" && !fn.IsArrowFunction {
				// Class constructors always have prototype
				if fn.IsClassConstructor {
					return vm.BooleanValue(true), nil
				}
				// For other functions, only report prototype if it's been accessed/created
				if fn.Properties != nil {
					if _, hasOwn := fn.Properties.GetOwn("prototype"); hasOwn {
						return vm.BooleanValue(true), nil
					}
				}
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
			// Bound functions have intrinsic own properties: name, length
			if propName == "name" || propName == "length" {
				return vm.BooleanValue(true), nil
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
		keyVal := args[0]
		if keyVal.Type() == vm.TypeSymbol {
			key := vm.NewSymbolKey(keyVal)
			if po := thisValue.AsPlainObject(); po != nil {
				if _, _, en, _, ok := po.GetOwnDescriptorByKey(key); ok {
					return vm.BooleanValue(en), nil
				}
				return vm.BooleanValue(false), nil
			}
			if dict := thisValue.AsDictObject(); dict != nil {
				// DictObject doesn’t support symbol keys
				return vm.BooleanValue(false), nil
			}
			if arr := thisValue.AsArray(); arr != nil {
				return vm.BooleanValue(false), nil
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
			// Per spec: length and callee are non-enumerable, numeric indices are enumerable
			if propName == "length" || propName == "callee" {
				return vm.BooleanValue(false), nil
			}
			if idx, err := strconv.Atoi(propName); err == nil && idx >= 0 && idx < argsObj.Length() {
				return vm.BooleanValue(true), nil
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

		// Step 3: Let O be ! ToObject(this value)
		// For objects, we check for @@toStringTag first

		// Determine the built-in tag based on type
		var builtinTag string
		switch thisValue.Type() {
		case vm.TypeBoolean:
			builtinTag = "Boolean"
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			builtinTag = "Number"
		case vm.TypeString:
			builtinTag = "String"
		case vm.TypeArray:
			builtinTag = "Array"
		case vm.TypeFunction, vm.TypeNativeFunction, vm.TypeClosure, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction:
			builtinTag = "Function"
		case vm.TypeRegExp:
			builtinTag = "RegExp"
		case vm.TypeMap:
			builtinTag = "Map"
		case vm.TypeSet:
			builtinTag = "Set"
		case vm.TypePromise:
			builtinTag = "Promise"
		case vm.TypeSymbol:
			builtinTag = "Symbol"
		case vm.TypeGenerator:
			builtinTag = "Generator"
		case vm.TypeArguments:
			builtinTag = "Arguments"
		case vm.TypeSharedArrayBuffer:
			builtinTag = "SharedArrayBuffer"
		case vm.TypeArrayBuffer:
			builtinTag = "ArrayBuffer"
		case vm.TypeTypedArray:
			builtinTag = "TypedArray" // Will be overridden by @@toStringTag from prototype
		case vm.TypeObject:
			// Check for wrapper objects with [[PrimitiveValue]] or [[ErrorData]]
			if plainObj := thisValue.AsPlainObject(); plainObj != nil {
				// Step 8: If O has [[ErrorData]], let builtinTag be "Error"
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
		default:
			builtinTag = "Object"
		}

		// Step 14-16: Check for @@toStringTag property
		// If the object has @@toStringTag and it's a string, use that instead
		if thisValue.IsObject() || thisValue.IsCallable() {
			var plainObj *vm.PlainObject
			switch thisValue.Type() {
			case vm.TypeObject:
				plainObj = thisValue.AsPlainObject()
			case vm.TypeClosure:
				if cl := thisValue.AsClosure(); cl != nil && cl.Fn != nil && cl.Fn.Properties != nil {
					plainObj = cl.Fn.Properties
				}
			case vm.TypeFunction:
				if fn := thisValue.AsFunction(); fn != nil && fn.Properties != nil {
					plainObj = fn.Properties
				}
			}

			if plainObj != nil {
				// Check for @@toStringTag symbol property
				if tag, ok := plainObj.GetOwnByKey(vm.NewSymbolKey(vmInstance.SymbolToStringTag)); ok {
					if tag.Type() == vm.TypeString {
						return vm.NewString("[object " + tag.ToString() + "]"), nil
					}
				}
			}
		}

		return vm.NewString("[object " + builtinTag + "]"), nil
	}))
	if v, ok := objectProto.GetOwn("toString"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("toString", v, &w, &e, &c)
	}

	objectProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.GetThis(), nil // Return this
	}))
	if v, ok := objectProto.GetOwn("valueOf"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("valueOf", v, &w, &e, &c)
	}

	objectProto.SetOwnNonEnumerable("toLocaleString", vm.NewNativeFunction(0, false, "toLocaleString", func(args []vm.Value) (vm.Value, error) {
		// Default implementation: call toString()
		thisValue := vmInstance.GetThis()
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
		thisValue := vmInstance.GetThis()
		obj := args[0]

		// Walk up the prototype chain of obj to see if thisValue is in it
		current := obj
		for {
			// Get the prototype of current object
			var proto vm.Value
			switch current.Type() {
			case vm.TypeObject:
				if plainObj := current.AsPlainObject(); plainObj != nil {
					proto = plainObj.GetPrototype()
				}
			case vm.TypeDictObject:
				if dictObj := current.AsDictObject(); dictObj != nil {
					proto = dictObj.GetPrototype()
				}
			case vm.TypeFunction, vm.TypeClosure, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeAsyncNativeFunction, vm.TypeBoundFunction:
				// All functions have FunctionPrototype as their prototype
				// Special case: Function.prototype itself has Object.prototype as its prototype
				if current.Is(vmInstance.FunctionPrototype) {
					proto = vmInstance.ObjectPrototype
				} else {
					proto = vmInstance.FunctionPrototype
				}
			case vm.TypeArray:
				proto = vmInstance.ArrayPrototype
			default:
				// No prototype for this type
				break
			}

			// If we couldn't get a prototype or reached null/undefined, we're done
			if proto.Type() == vm.TypeNull || proto.Type() == vm.TypeUndefined || proto.Type() == 0 {
				break
			}

			// Check if this prototype is the one we're looking for
			if proto.Is(thisValue) {
				return vm.BooleanValue(true), nil
			}

			current = proto
		}

		return vm.BooleanValue(false), nil
	}))
	if v, ok := objectProto.GetOwn("isPrototypeOf"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("isPrototypeOf", v, &w, &e, &c)
	}

	// __proto__ accessor property (ES6 B.2.2.1)
	// Getter: Object.getPrototypeOf(this)
	protoGetter := vm.NewNativeFunction(0, false, "get __proto__", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeUndefined || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot read property '__proto__' of " + thisValue.TypeName())
		}
		// Get prototype from various object types
		switch thisValue.Type() {
		case vm.TypeObject:
			if po := thisValue.AsPlainObject(); po != nil {
				return po.GetPrototype(), nil
			}
		case vm.TypeDictObject:
			if d := thisValue.AsDictObject(); d != nil {
				return d.GetPrototype(), nil
			}
		case vm.TypeArray:
			return vmInstance.ArrayPrototype, nil
		case vm.TypeFunction:
			return vmInstance.FunctionPrototype, nil
		case vm.TypeClosure:
			return vmInstance.FunctionPrototype, nil
		case vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction:
			return vmInstance.FunctionPrototype, nil
		case vm.TypeRegExp:
			return vmInstance.RegExpPrototype, nil
		case vm.TypeMap:
			return vmInstance.MapPrototype, nil
		case vm.TypeSet:
			return vmInstance.SetPrototype, nil
		case vm.TypeProxy:
			// For proxy, get the target's prototype through the proxy
			proxy := thisValue.AsProxy()
			if proxy.Revoked {
				return vm.Undefined, vmInstance.NewTypeError("Cannot read property '__proto__' of a revoked Proxy")
			}
			target := proxy.Target()
			if po := target.AsPlainObject(); po != nil {
				return po.GetPrototype(), nil
			}
			return vm.Null, nil
		case vm.TypeArrayBuffer:
			return vmInstance.ObjectPrototype, nil
		case vm.TypeSharedArrayBuffer:
			return vmInstance.SharedArrayBufferPrototype, nil
		case vm.TypeTypedArray:
			// Delegate to objectGetPrototypeOfWithVM for typed arrays
			return objectGetPrototypeOfWithVM(vmInstance, []vm.Value{thisValue})
		}
		// For primitives, return their prototype
		switch thisValue.Type() {
		case vm.TypeString:
			return vmInstance.StringPrototype, nil
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return vmInstance.NumberPrototype, nil
		case vm.TypeBoolean:
			return vmInstance.BooleanPrototype, nil
		case vm.TypeSymbol:
			return vmInstance.SymbolPrototype, nil
		}
		return vm.Null, nil
	})
	// Setter: Object.setPrototypeOf(this, value)
	protoSetter := vm.NewNativeFunction(1, false, "set __proto__", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeUndefined || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot set property '__proto__' of " + thisValue.TypeName())
		}
		if len(args) == 0 {
			return vm.Undefined, nil
		}
		protoArg := args[0]
		// Prototype must be object or null
		if protoArg.Type() != vm.TypeNull && !protoArg.IsObject() {
			// Non-object, non-null values are silently ignored per spec
			return vm.Undefined, nil
		}
		// Set prototype on object types
		switch thisValue.Type() {
		case vm.TypeObject:
			if po := thisValue.AsPlainObject(); po != nil {
				if !po.SetPrototype(protoArg) {
					// SetPrototype returns false if object is non-extensible and prototype differs
					// In strict mode, this would throw TypeError
					// Per spec B.2.2.1.2, we return undefined (silent failure in non-strict)
					return vm.Undefined, nil
				}
			}
		case vm.TypeDictObject:
			if d := thisValue.AsDictObject(); d != nil {
				if !d.SetPrototype(protoArg) {
					return vm.Undefined, nil
				}
			}
		case vm.TypeArray:
			// Arrays have a fixed prototype (Array.prototype)
			// Setting __proto__ on an array is silently ignored per spec
		default:
			// Other object types (functions, etc.) - prototype change is typically not allowed
			// Silently ignore per spec
		}
		return vm.Undefined, nil
	})
	// Define as accessor property: enumerable=false, configurable=true
	e, c := false, true
	objectProto.DefineAccessorProperty("__proto__", protoGetter, true, protoSetter, true, &e, &c)

	// __defineGetter__ (ES6 B.2.2.2) - legacy method to define a getter
	defineGetterFunc := vm.NewNativeFunction(2, false, "__defineGetter__", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() == vm.TypeUndefined || thisValue.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("__defineGetter__ requires 2 arguments")
		}
		// ToPropertyKey
		propName := args[0].ToString()
		getter := args[1]
		if !getter.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("__defineGetter__ getter must be a function")
		}
		// Define accessor property based on object type
		// Per ES6 B.2.2.2: enumerable: true, configurable: true
		en, conf := true, true
		switch thisValue.Type() {
		case vm.TypeObject:
			thisValue.AsPlainObject().DefineAccessorProperty(propName, getter, true, vm.Undefined, false, &en, &conf)
		case vm.TypeArray:
			thisValue.AsArray().DefineAccessorProperty(propName, getter, true, vm.Undefined, false, &en, &conf)
		default:
			if po := thisValue.AsPlainObject(); po != nil {
				po.DefineAccessorProperty(propName, getter, true, vm.Undefined, false, &en, &conf)
			} else {
				return vm.Undefined, vmInstance.NewTypeError("__defineGetter__ called on non-object")
			}
		}
		return vm.Undefined, nil
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
		// ToPropertyKey
		propName := args[0].ToString()
		setter := args[1]
		if !setter.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("__defineSetter__ setter must be a function")
		}
		// Define accessor property based on object type
		// Per ES6 B.2.2.3: enumerable: true, configurable: true
		en, conf := true, true
		switch thisValue.Type() {
		case vm.TypeObject:
			thisValue.AsPlainObject().DefineAccessorProperty(propName, vm.Undefined, false, setter, true, &en, &conf)
		case vm.TypeArray:
			thisValue.AsArray().DefineAccessorProperty(propName, vm.Undefined, false, setter, true, &en, &conf)
		default:
			if po := thisValue.AsPlainObject(); po != nil {
				po.DefineAccessorProperty(propName, vm.Undefined, false, setter, true, &en, &conf)
			} else {
				return vm.Undefined, vmInstance.NewTypeError("__defineSetter__ called on non-object")
			}
		}
		return vm.Undefined, nil
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
		propName := args[0].ToString()
		// Walk up the prototype chain looking for accessor
		current := thisValue
		for {
			var po *vm.PlainObject
			switch current.Type() {
			case vm.TypeObject:
				po = current.AsPlainObject()
			case vm.TypeArray:
				arr := current.AsArray()
				// Check if array has accessor property
				if getter, _, _, _, isAccessor := arr.GetOwnAccessor(propName); isAccessor {
					return getter, nil
				}
				// Check if array has data property (if so, return undefined)
				if _, hasData := arr.GetOwn(propName); hasData {
					return vm.Undefined, nil
				}
				// Move to array prototype
				current = vmInstance.ArrayPrototype
				continue
			default:
				po = current.AsPlainObject()
			}
			if po != nil {
				// Check if it's an accessor property
				if getter, _, _, _, isAccessor := po.GetOwnAccessor(propName); isAccessor {
					return getter, nil
				}
				// Check if it's a data property (if so, return undefined per spec)
				if _, hasData := po.GetOwn(propName); hasData {
					return vm.Undefined, nil
				}
				// Move up prototype chain
				proto := po.GetPrototype()
				if proto.Type() == vm.TypeNull || proto.Type() == vm.TypeUndefined || proto.Type() == 0 {
					break
				}
				current = proto
			} else {
				break
			}
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
		propName := args[0].ToString()
		// Walk up the prototype chain looking for accessor
		current := thisValue
		for {
			var po *vm.PlainObject
			switch current.Type() {
			case vm.TypeObject:
				po = current.AsPlainObject()
			case vm.TypeArray:
				arr := current.AsArray()
				// Check if array has accessor property
				if _, setter, _, _, isAccessor := arr.GetOwnAccessor(propName); isAccessor {
					return setter, nil
				}
				// Check if array has data property (if so, return undefined)
				if _, hasData := arr.GetOwn(propName); hasData {
					return vm.Undefined, nil
				}
				// Move to array prototype
				current = vmInstance.ArrayPrototype
				continue
			default:
				po = current.AsPlainObject()
			}
			if po != nil {
				// Check if it's an accessor property
				if _, setter, _, _, isAccessor := po.GetOwnAccessor(propName); isAccessor {
					return setter, nil
				}
				// Check if it's a data property (if so, return undefined per spec)
				if _, hasData := po.GetOwn(propName); hasData {
					return vm.Undefined, nil
				}
				// Move up prototype chain
				proto := po.GetPrototype()
				if proto.Type() == vm.TypeNull || proto.Type() == vm.TypeUndefined || proto.Type() == 0 {
					break
				}
				current = proto
			} else {
				break
			}
		}
		return vm.Undefined, nil
	})
	objectProto.SetOwnNonEnumerable("__lookupSetter__", lookupSetterFunc)

	// Create Object constructor (length=1 per spec)
	objectCtor := vm.NewNativeFunction(1, true, "Object", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewObject(vm.NewValueFromPlainObject(objectProto)), nil
		}
		arg := args[0]

		// If already an object type, return as-is
		if arg.IsObject() || arg.Type() == vm.TypeArray || arg.Type() == vm.TypeRegExp ||
			arg.Type() == vm.TypeMap || arg.Type() == vm.TypeSet || arg.Type() == vm.TypeProxy {
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
			// Add valueOf method that returns the primitive
			wrapper.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
				return arg, nil
			}))
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			// Create Number wrapper object
			wrapper := vm.NewObject(vmInstance.NumberPrototype).AsPlainObject()
			wrapper.SetOwnNonEnumerable("[[PrimitiveValue]]", arg)
			wrapper.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
				return arg, nil
			}))
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeString:
			// Create String wrapper object
			wrapper := vm.NewObject(vmInstance.StringPrototype).AsPlainObject()
			wrapper.SetOwnNonEnumerable("[[PrimitiveValue]]", arg)
			wrapper.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
				return arg, nil
			}))
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeBoolean:
			// Create Boolean wrapper object
			wrapper := vm.NewObject(vmInstance.BooleanPrototype).AsPlainObject()
			wrapper.SetOwnNonEnumerable("[[PrimitiveValue]]", arg)
			wrapper.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
				return arg, nil
			}))
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeSymbol:
			// Create Symbol wrapper object
			wrapper := vm.NewObject(vmInstance.SymbolPrototype).AsPlainObject()
			wrapper.SetOwnNonEnumerable("[[PrimitiveValue]]", arg)
			wrapper.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
				return arg, nil
			}))
			return vm.NewValueFromPlainObject(wrapper), nil

		default:
			// Fallback: create empty object
			return vm.NewObject(vm.NewValueFromPlainObject(objectProto)), nil
		}
	})

	// Make it a proper constructor with static methods
	if ctorObj := objectCtor.AsNativeFunction(); ctorObj != nil {
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
				keyResult, err := vmInstance.Call(callbackfn, vm.Undefined, []vm.Value{value, vm.NumberValue(float64(k))})
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
		if plainObj := obj.AsPlainObject(); plainObj != nil {
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
	if !propertiesDesc.IsObject() && !propertiesDesc.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError("Properties must be an object")
	}

	// Get the plain object to define properties on
	plainObj := obj.AsPlainObject()
	if plainObj == nil {
		return vm.Undefined, vmInstance.NewTypeError("Cannot define properties on non-plain object")
	}

	// Helper to check if property exists and get its value using GetProperty (calls getters)
	hasAndGetProperty := func(descObj vm.Value, propName string) (vm.Value, bool, error) {
		// Check if property exists (including prototype chain)
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

		switch descObj.Type() {
		case vm.TypeObject:
			if po := descObj.AsPlainObject(); po != nil {
				exists = po.Has(propName)
			}
		case vm.TypeDictObject:
			if do := descObj.AsDictObject(); do != nil {
				_, exists = do.Get(propName)
			}
		case vm.TypeFunction:
			fn := descObj.AsFunction()
			if fn != nil {
				if fn.Properties != nil && fn.Properties.Has(propName) {
					exists = true
				} else {
					exists = checkFunctionPrototype()
				}
			}
		case vm.TypeClosure:
			cl := descObj.AsClosure()
			if cl != nil {
				if cl.Properties != nil && cl.Properties.Has(propName) {
					exists = true
				} else {
					exists = checkFunctionPrototype()
				}
			}
		case vm.TypeBoundFunction:
			bf := descObj.AsBoundFunction()
			if bf != nil {
				if bf.Properties != nil && bf.Properties.Has(propName) {
					exists = true
				} else {
					exists = checkFunctionPrototype()
				}
			}
		case vm.TypeNativeFunctionWithProps:
			nfp := descObj.AsNativeFunctionWithProps()
			if nfp != nil {
				if nfp.Properties != nil && nfp.Properties.Has(propName) {
					exists = true
				} else {
					exists = checkFunctionPrototype()
				}
			}
		case vm.TypeRegExp:
			regex := descObj.AsRegExpObject()
			if regex != nil {
				if regex.Properties != nil && regex.Properties.Has(propName) {
					exists = true
				} else if vmInstance.RegExpPrototype.IsObject() {
					proto := vmInstance.RegExpPrototype.AsPlainObject()
					exists = proto.Has(propName)
				}
			}
		case vm.TypeArray:
			arr := descObj.AsArray()
			if arr != nil {
				if _, ok := arr.GetOwn(propName); ok {
					exists = true
				} else if vmInstance.ArrayPrototype.IsObject() {
					proto := vmInstance.ArrayPrototype.AsPlainObject()
					exists = proto.Has(propName)
				}
			}
		case vm.TypeArguments:
			args := descObj.AsArguments()
			if args != nil {
				if args.HasNamedProp(propName) {
					exists = true
				} else if vmInstance.ObjectPrototype.IsObject() {
					proto := vmInstance.ObjectPrototype.AsPlainObject()
					exists = proto.Has(propName)
				}
			}
		}

		if !exists {
			return vm.Undefined, false, nil
		}

		// Property exists, use GetProperty to call getters
		val, err := vmInstance.GetProperty(descObj, propName)
		if err != nil {
			return vm.Undefined, false, err
		}
		return val, true, nil
	}

	// Get keys from the properties descriptor
	var keys []string
	switch propertiesDesc.Type() {
	case vm.TypeObject:
		if po := propertiesDesc.AsPlainObject(); po != nil {
			for _, key := range po.OwnKeys() {
				if _, _, enumerable, _, ok := po.GetOwnDescriptor(key); ok && enumerable {
					keys = append(keys, key)
				}
			}
		}
	case vm.TypeArray:
		if arr := propertiesDesc.AsArray(); arr != nil {
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
		if po := propertiesDesc.AsPlainObject(); po != nil {
			for _, key := range po.OwnKeys() {
				if _, _, enumerable, _, ok := po.GetOwnDescriptor(key); ok && enumerable {
					keys = append(keys, key)
				}
			}
		}
	}

	// Process each property
	for _, key := range keys {
		// Get the property descriptor object
		propDesc, err := vmInstance.GetProperty(propertiesDesc, key)
		if err != nil {
			return vm.Undefined, err
		}

		// propDesc should be an object
		if !propDesc.IsObject() && !propDesc.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Property description must be an object: " + key)
		}

		// Extract descriptor properties using hasAndGetProperty (calls getters)
		var value vm.Value
		var writable, enumFlag, configurable bool
		var hasValue, hasWritable, hasEnumerable, hasConfigurable bool
		var getter, setter vm.Value
		var hasGetter, hasSetter bool

		// Check for 'value' property
		if v, exists, err := hasAndGetProperty(propDesc, "value"); err != nil {
			return vm.Undefined, err
		} else if exists {
			hasValue = true
			value = v
		}

		// Check for 'writable' property
		if v, exists, err := hasAndGetProperty(propDesc, "writable"); err != nil {
			return vm.Undefined, err
		} else if exists {
			hasWritable = true
			writable = v.IsTruthy()
		}

		// Check for 'enumerable' property
		if v, exists, err := hasAndGetProperty(propDesc, "enumerable"); err != nil {
			return vm.Undefined, err
		} else if exists {
			hasEnumerable = true
			enumFlag = v.IsTruthy()
		}

		// Check for 'configurable' property
		if v, exists, err := hasAndGetProperty(propDesc, "configurable"); err != nil {
			return vm.Undefined, err
		} else if exists {
			hasConfigurable = true
			configurable = v.IsTruthy()
		}

		// Check for 'get' property
		if v, exists, err := hasAndGetProperty(propDesc, "get"); err != nil {
			return vm.Undefined, err
		} else if exists {
			hasGetter = true
			getter = v
			// Validate getter
			if getter.Type() != vm.TypeUndefined && !getter.IsCallable() {
				return vm.Undefined, vmInstance.NewTypeError("Getter must be a function")
			}
		}

		// Check for 'set' property
		if v, exists, err := hasAndGetProperty(propDesc, "set"); err != nil {
			return vm.Undefined, err
		} else if exists {
			hasSetter = true
			setter = v
			// Validate setter
			if setter.Type() != vm.TypeUndefined && !setter.IsCallable() {
				return vm.Undefined, vmInstance.NewTypeError("Setter must be a function")
			}
		}

		// Validate: can't have both data and accessor properties
		if (hasValue || hasWritable) && (hasGetter || hasSetter) {
			return vm.Undefined, vmInstance.NewTypeError("Invalid property descriptor. Cannot both specify accessors and a value or writable attribute")
		}

		// Apply defaults
		if !hasValue && !(hasGetter || hasSetter) {
			value = vm.Undefined
		}

		// Use pointers for DefineOwnProperty
		var wPtr, ePtr, cPtr *bool
		if hasWritable {
			wPtr = &writable
		} else if !(hasGetter || hasSetter) {
			f := false
			wPtr = &f
		}
		if hasEnumerable {
			ePtr = &enumFlag
		} else {
			f := false
			ePtr = &f
		}
		if hasConfigurable {
			cPtr = &configurable
		} else {
			f := false
			cPtr = &f
		}

		// Use accessor path if getter or setter is specified
		if hasGetter || hasSetter {
			plainObj.DefineAccessorProperty(key, getter, hasGetter, setter, hasSetter, ePtr, cPtr)
		} else {
			plainObj.DefineOwnProperty(key, value, wPtr, ePtr, cPtr)
		}
	}

	return obj, nil
}

func objectDefinePropertiesWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) < 2 {
		return vm.Undefined, vmInstance.NewTypeError("Object.defineProperties requires at least 2 arguments")
	}

	obj := args[0]
	if !obj.IsObject() {
		return vm.Undefined, vmInstance.NewTypeError("Object.defineProperties called on non-object")
	}

	return objectDefinePropertiesImpl(vmInstance, obj, args[1])
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
	// Primitives (non-object/non-callable) have no enumerable own keys
	if !obj.IsObject() && !obj.IsCallable() {
		return vm.NewArray(), nil
	}

	keys := vm.NewArray()
	keysArray := keys.AsArray()

	// Handle Proxy objects - call ownKeys trap if present
	if obj.Type() == vm.TypeProxy {
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot get keys of revoked Proxy")
		}

		// Check for ownKeys trap
		if ownKeysTrap, ok := proxy.Handler().AsPlainObject().GetOwn("ownKeys"); ok {
			// Validate trap is callable
			if !ownKeysTrap.IsFunction() {
				return vm.Undefined, vmInstance.NewTypeError("'ownKeys' on proxy: trap is not a function")
			}

			// Call handler.ownKeys(target)
			result, err := vmInstance.Call(ownKeysTrap, proxy.Handler(), []vm.Value{proxy.Target()})
			if err != nil {
				return vm.Undefined, err
			}

			// Result must be an array-like object
			if result.Type() != vm.TypeArray {
				return vm.Undefined, vmInstance.NewTypeError("'ownKeys' on proxy: trap result must be an array")
			}

			// Use keys from trap result (trap is responsible for filtering)
			resultArray := result.AsArray()
			for i := 0; i < resultArray.Length(); i++ {
				key := resultArray.Get(i)
				// Object.keys only returns string keys (not symbols)
				if key.Type() == vm.TypeString {
					keysArray.Append(key)
				}
			}
			return keys, nil
		}

		// No ownKeys trap, delegate to target
		return objectKeysWithVM(vmInstance, []vm.Value{proxy.Target()})
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
		for i := 0; i < arrObj.Length(); i++ {
			keysArray.Append(vm.NewString(strconv.Itoa(i)))
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
	}

	return keys, nil
}

// (Removed duplicate alternate implementations of objectValuesImpl and objectEntriesImpl)

func objectGetPrototypeOfWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, nil
	}

	obj := args[0]

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
		// For strings, return String.prototype if available
		return vm.Null, nil // TODO: Return proper String.prototype
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
		// For NativeFunctionWithProps (like Function.prototype), return its internal prototype
		nfp := obj.AsNativeFunctionWithProps()
		if nfp != nil && nfp.Properties != nil {
			return nfp.Properties.GetPrototype(), nil
		}
		return vm.Null, nil
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

		// Check if handler has a getPrototypeOf trap
		if trap, ok := proxy.Handler().AsPlainObject().GetOwn("getPrototypeOf"); ok {
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
		// For ArrayBuffers, return Object.prototype (ArrayBuffer doesn't have its own stored prototype)
		return vmInstance.ObjectPrototype, nil
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

		// Check for setPrototypeOf trap
		if setProtoTrap, ok := proxy.Handler().AsPlainObject().GetOwn("setPrototypeOf"); ok {
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
	if plainObj := obj.AsPlainObject(); plainObj != nil && plainObj.IsModuleNamespace() {
		// Namespace prototype is always null
		if proto.Type() == vm.TypeNull {
			return obj, nil // Success - proto matches
		}
		return vm.Undefined, vmInstance.NewTypeError("Cannot set prototype of immutable prototype exotic object")
	}

	// Set the prototype based on object type
	success := true
	switch obj.Type() {
	case vm.TypeObject:
		if plainObj := obj.AsPlainObject(); plainObj != nil {
			success = plainObj.SetPrototype(proto)
		} else if dictObj := obj.AsDictObject(); dictObj != nil {
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
		if plainObj := obj.AsPlainObject(); plainObj != nil {
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
	if !obj.IsObject() && !obj.IsCallable() {
		return vm.NewArray(), nil
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
		for i := 0; i < arrObj.Length(); i++ {
			valuesArray.Append(arrObj.Get(i))
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
	if !obj.IsObject() && !obj.IsCallable() {
		return vm.NewArray(), nil
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
		for i := 0; i < arrObj.Length(); i++ {
			entry := vm.NewArray()
			entry.AsArray().Append(vm.NewString(strconv.Itoa(i)))
			entry.AsArray().Append(arrObj.Get(i))
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
		if po := obj.AsPlainObject(); po != nil {
			// OwnPropertyNames returns ALL own string property names including non-enumerable
			for _, k := range po.OwnPropertyNames() {
				arrObj.Append(vm.NewString(k))
			}
		} else if d := obj.AsDictObject(); d != nil {
			// DictObject.OwnPropertyNames returns all property names
			for _, k := range d.OwnPropertyNames() {
				arrObj.Append(vm.NewString(k))
			}
		}
	case vm.TypeArray:
		a := obj.AsArray()
		for i := 0; i < a.Length(); i++ {
			arrObj.Append(vm.NewString(strconv.Itoa(i)))
		}
		arrObj.Append(vm.NewString("length"))
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

		// If no prototype was found in Properties, add it at the end
		if !hasPrototype {
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

		if !hasPrototype {
			arrObj.Append(vm.NewString("prototype"))
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
	if po := obj.AsPlainObject(); po != nil {
		for _, s := range po.OwnSymbolKeys() {
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
	}
	// DictObject does not support symbols; returns empty array
	return arr, nil
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
	if po := obj.AsPlainObject(); po != nil {
		// Strings first
		for _, k := range po.OwnKeys() {
			outArr.Append(vm.NewString(k))
		}
		// Then symbols
		for _, s := range po.OwnSymbolKeys() {
			outArr.Append(s)
		}
	} else if d := obj.AsDictObject(); d != nil {
		for _, k := range d.OwnKeys() {
			outArr.Append(vm.NewString(k))
		}
		// DictObject: no symbols yet
	} else if a := obj.AsArray(); a != nil {
		for i := 0; i < a.Length(); i++ {
			outArr.Append(vm.NewString(strconv.Itoa(i)))
		}
		outArr.Append(vm.NewString("length"))
	}
	return out, nil
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
		if plainObj := source.AsPlainObject(); plainObj != nil {
			for _, key := range plainObj.OwnKeys() {
				value, _ := plainObj.GetOwn(key)
				// Set on target
				if targetPlain := target.AsPlainObject(); targetPlain != nil {
					targetPlain.SetOwnNonEnumerable(key, value)
				} else if targetDict := target.AsDictObject(); targetDict != nil {
					targetDict.SetOwn(key, value)
				}
			}
		} else if dictObj := source.AsDictObject(); dictObj != nil {
			for _, key := range dictObj.OwnKeys() {
				value, _ := dictObj.GetOwn(key)
				// Set on target
				if targetPlain := target.AsPlainObject(); targetPlain != nil {
					targetPlain.SetOwnNonEnumerable(key, value)
				} else if targetDict := target.AsDictObject(); targetDict != nil {
					targetDict.SetOwn(key, value)
				}
			}
		} else if arrObj := source.AsArray(); arrObj != nil {
			// For arrays, copy indexed properties
			for i := 0; i < arrObj.Length(); i++ {
				key := strconv.Itoa(i)
				value := arrObj.Get(i)
				// Set on target
				if targetPlain := target.AsPlainObject(); targetPlain != nil {
					targetPlain.SetOwnNonEnumerable(key, value)
				} else if targetDict := target.AsDictObject(); targetDict != nil {
					targetDict.SetOwn(key, value)
				}
			}
			// Also copy length property
			if targetPlain := target.AsPlainObject(); targetPlain != nil {
				targetPlain.SetOwnNonEnumerable("length", vm.NumberValue(float64(arrObj.Length())))
			} else if targetDict := target.AsDictObject(); targetDict != nil {
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
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		if isSymbol {
			return vm.BooleanValue(plainObj.HasOwnByKey(vm.NewSymbolKey(keyVal))), nil
		}
		_, hasOwn := plainObj.GetOwn(keyVal.ToString())
		return vm.BooleanValue(hasOwn), nil
	}
	if dictObj := obj.AsDictObject(); dictObj != nil {
		if isSymbol {
			return vm.BooleanValue(false), nil
		}
		_, hasOwn := dictObj.GetOwn(keyVal.ToString())
		return vm.BooleanValue(hasOwn), nil
	}
	if arrObj := obj.AsArray(); arrObj != nil {
		if isSymbol {
			return vm.BooleanValue(false), nil
		}
		propName := keyVal.ToString()
		// For arrays, check if it's a valid index or 'length'
		if propName == "length" {
			return vm.BooleanValue(true), nil
		}
		// Check numeric indices
		if index, err := strconv.Atoi(propName); err == nil {
			return vm.BooleanValue(index >= 0 && index < arrObj.Length()), nil
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
	if arr := iterable.AsArray(); arr != nil {
		for i := 0; i < arr.Length(); i++ {
			entry := arr.Get(i)

			// Each entry should be an array-like with at least 2 elements
			if entryArr := entry.AsArray(); entryArr != nil && entryArr.Length() >= 2 {
				key := entryArr.Get(0).ToString()
				value := entryArr.Get(1)
				resultObj.SetOwnNonEnumerable(key, value)
			}
		}
	}
	// TODO: Support other iterables when iterator protocol is implemented

	return result, nil
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

		// Check for defineProperty trap
		if defineTrap, ok := proxy.Handler().AsPlainObject().GetOwn("defineProperty"); ok {
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
		obj.Type() == vm.TypeBoundFunction
	if !isObjectLike {
		return vm.Undefined, vmInstance.NewTypeError("Object.defineProperty called on non-object")
	}

	// Module Namespace Exotic Object [[DefineOwnProperty]] behavior (ECMAScript 10.4.6.7)
	// Returns true if no change is requested, false otherwise
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
			if po := obj.AsPlainObject(); po != nil {
				exists = po.Has(propName)
			}
		case vm.TypeDictObject:
			if do := obj.AsDictObject(); do != nil {
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

	// Handle BoundFunction first (before AsPlainObject which would panic)
	if obj.Type() == vm.TypeBoundFunction {
		bf := obj.AsBoundFunction()
		if bf != nil {
			if bf.Properties == nil {
				bf.Properties = vm.NewObject(vm.Undefined).AsPlainObject()
			}
			if hasGetter || hasSetter {
				if keyIsSymbol {
					bf.Properties.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					bf.Properties.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					bf.Properties.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					bf.Properties.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
		}
		return obj, nil
	}

	// Define the property with attributes (on plain objects only for now)
	if plainObj := obj.AsPlainObject(); plainObj != nil {
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
			// Non-configurable: cannot change configurable or enumerable
			if configurablePtr != nil && *configurablePtr != c0 {
				return obj, nil
			}
			if enumerablePtr != nil && *enumerablePtr != e0 {
				return obj, nil
			}
			// If data non-writable cannot make writable true
			if !isAccessor0 && !w0 && writablePtr != nil && *writablePtr {
				return obj, nil
			}
			// Disallow converting kind when not configurable
			if isAccessor0 && !(hasGetter || hasSetter) {
				return obj, nil
			}
			if !isAccessor0 && (hasGetter || hasSetter) {
				return obj, nil
			}
		}
		if hasGetter || hasSetter {
			// Accessor path
			if keyIsSymbol {
				plainObj.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
			} else {
				plainObj.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
			}
		} else {
			if keyIsSymbol {
				plainObj.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
			} else {
				plainObj.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
			}
		}
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		// DictObject has no attributes; set value only for string keys; symbols unsupported
		if !keyIsSymbol {
			dictObj.SetOwn(propName, value)
		}
	} else if obj.Type() == vm.TypeNativeFunctionWithProps {
		// NativeFunctionWithProps (like Function.prototype) stores properties in Properties
		nfp := obj.AsNativeFunctionWithProps()
		if nfp != nil && nfp.Properties != nil {
			if hasGetter || hasSetter {
				if keyIsSymbol {
					nfp.Properties.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					nfp.Properties.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					nfp.Properties.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					nfp.Properties.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
		}
	} else if obj.Type() == vm.TypeFunction {
		// Functions store additional properties in Properties field
		fn := obj.AsFunction()
		if fn != nil {
			if fn.Properties == nil {
				fn.Properties = vm.NewObject(vm.Undefined).AsPlainObject()
			}
			if hasGetter || hasSetter {
				if keyIsSymbol {
					fn.Properties.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					fn.Properties.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					fn.Properties.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					fn.Properties.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
		}
	} else if obj.Type() == vm.TypeClosure {
		// Closures store additional properties in their function's Properties field
		cl := obj.AsClosure()
		if cl != nil && cl.Fn != nil {
			if cl.Fn.Properties == nil {
				cl.Fn.Properties = vm.NewObject(vm.Undefined).AsPlainObject()
			}
			if hasGetter || hasSetter {
				if keyIsSymbol {
					cl.Fn.Properties.DefineAccessorPropertyByKey(vm.NewSymbolKey(propSym), getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				} else {
					cl.Fn.Properties.DefineAccessorProperty(propName, getter, hasGetter, setter, hasSetter, enumerablePtr, configurablePtr)
				}
			} else {
				if keyIsSymbol {
					cl.Fn.Properties.DefineOwnPropertyByKey(vm.NewSymbolKey(propSym), value, writablePtr, enumerablePtr, configurablePtr)
				} else {
					cl.Fn.Properties.DefineOwnProperty(propName, value, writablePtr, enumerablePtr, configurablePtr)
				}
			}
		}
	}

	return obj, nil
}

func objectGetOwnPropertyDescriptorWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) < 2 {
		return vm.Undefined, nil
	}

	obj := args[0]
	var keyIsSymbol bool
	var propName string
	var propSym vm.Value
	if args[1].Type() == vm.TypeSymbol {
		keyIsSymbol = true
		propSym = args[1]
	} else {
		propName = args[1].ToString()
	}

	// Handle Proxy objects
	if obj.Type() == vm.TypeProxy {
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, vmInstance.NewTypeError("Cannot get property descriptor on revoked Proxy")
		}

		// Check for getOwnPropertyDescriptor trap
		if getTrap, ok := proxy.Handler().AsPlainObject().GetOwn("getOwnPropertyDescriptor"); ok {
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

			// Result must be undefined or an object
			if result.Type() != vm.TypeUndefined && !result.IsObject() {
				return vm.Undefined, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap result must be an object or undefined")
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
				if g.Type() != vm.TypeUndefined {
					descriptor.SetOwn("get", g)
				}
				if s.Type() != vm.TypeUndefined {
					descriptor.SetOwn("set", s)
				}
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
				if g.Type() != vm.TypeUndefined {
					descriptor.SetOwn("get", g)
				}
				if s.Type() != vm.TypeUndefined {
					descriptor.SetOwn("set", s)
				}
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
				if g.Type() != vm.TypeUndefined {
					descriptor.SetOwn("get", g)
				}
				if s.Type() != vm.TypeUndefined {
					descriptor.SetOwn("set", s)
				}
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
				if g.Type() != vm.TypeUndefined {
					descriptor.SetOwn("get", g)
				}
				if s.Type() != vm.TypeUndefined {
					descriptor.SetOwn("set", s)
				}
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
		isFrozen := arrObj.IsFrozen()
		// For arrays, check if it's a valid index or 'length'
		if propName == "length" {
			value = vm.NumberValue(float64(arrObj.Length()))
			// length is non-enumerable, non-configurable; writable unless frozen
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", value)
			descriptor.SetOwn("writable", vm.BooleanValue(!isFrozen))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(false))
			return vm.NewValueFromPlainObject(descriptor), nil
		} else if index, err := strconv.Atoi(propName); err == nil && index >= 0 && index < arrObj.Length() {
			value = arrObj.Get(index)
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", value)
			// When frozen, elements are not writable and not configurable
			descriptor.SetOwn("writable", vm.BooleanValue(!isFrozen))
			descriptor.SetOwn("enumerable", vm.BooleanValue(true))
			descriptor.SetOwn("configurable", vm.BooleanValue(!isFrozen))
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
		if g, s, e, c, ok := func() (vm.Value, vm.Value, bool, bool, bool) {
			if keyIsSymbol {
				return plainObj.GetOwnAccessorByKey(vm.NewSymbolKey(propSym))
			}
			return plainObj.GetOwnAccessor(propName)
		}(); ok {
			// Accessor descriptor
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			if g.Type() != vm.TypeUndefined {
				descriptor.SetOwn("get", g)
			}
			if s.Type() != vm.TypeUndefined {
				descriptor.SetOwn("set", s)
			}
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
				if g.Type() != vm.TypeUndefined {
					descriptor.SetOwn("get", g)
				}
				if s.Type() != vm.TypeUndefined {
					descriptor.SetOwn("set", s)
				}
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
		// Check for numeric index
		if index, err := strconv.Atoi(propName); err == nil && index >= 0 && index < argsObj.Length() {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", argsObj.Get(index))
			descriptor.SetOwn("writable", vm.BooleanValue(true))
			descriptor.SetOwn("enumerable", vm.BooleanValue(true))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
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
		if propName == "name" {
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NewString(bf.Name))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		if propName == "length" {
			// Bound functions have reduced length by the number of bound arguments
			var originalLength int
			switch bf.OriginalFunction.Type() {
			case vm.TypeFunction:
				originalLength = bf.OriginalFunction.AsFunction().Length
			case vm.TypeClosure:
				originalLength = bf.OriginalFunction.AsClosure().Fn.Length
			case vm.TypeNativeFunction:
				originalLength = bf.OriginalFunction.AsNativeFunction().Arity
			case vm.TypeNativeFunctionWithProps:
				originalLength = bf.OriginalFunction.AsNativeFunctionWithProps().Arity
			}
			boundLength := originalLength - len(bf.PartialArgs)
			if boundLength < 0 {
				boundLength = 0
			}
			descriptor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			descriptor.SetOwn("value", vm.NumberValue(float64(boundLength)))
			descriptor.SetOwn("writable", vm.BooleanValue(false))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
	}

	// Handle RegExp intrinsic property: lastIndex
	// Per ECMAScript spec: {value: 0, writable: true, enumerable: false, configurable: false}
	if obj.Type() == vm.TypeRegExp {
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
		if po := obj.AsPlainObject(); po != nil {
			stringKeys = po.OwnPropertyNames()
			symbolKeys = po.OwnSymbolKeys()
		} else if d := obj.AsDictObject(); d != nil {
			stringKeys = d.OwnPropertyNames()
		}
	case vm.TypeArray:
		arr := obj.AsArray()
		for i := 0; i < arr.Length(); i++ {
			stringKeys = append(stringKeys, strconv.Itoa(i))
		}
		stringKeys = append(stringKeys, "length")
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
		if nfp := obj.AsNativeFunctionWithProps(); nfp != nil && nfp.Properties != nil {
			for _, k := range nfp.Properties.OwnPropertyNames() {
				if k != "length" && k != "name" {
					stringKeys = append(stringKeys, k)
				}
			}
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

		// Check for isExtensible trap
		if extTrap, ok := proxy.Handler().AsPlainObject().GetOwn("isExtensible"); ok {
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

			// Convert to boolean
			return vm.BooleanValue(result.IsTruthy()), nil
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
	if obj.IsObject() {
		if plainObj := obj.AsPlainObject(); plainObj != nil {
			return vm.BooleanValue(plainObj.IsExtensible()), nil
		}
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

		// Check for preventExtensions trap
		if prevTrap, ok := proxy.Handler().AsPlainObject().GetOwn("preventExtensions"); ok {
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

			// If trap returns falsy, throw TypeError
			if result.IsFalsey() {
				return vm.Undefined, vmInstance.NewTypeError("'preventExtensions' on proxy: trap returned falsish")
			}

			return obj, nil
		}

		// No trap, delegate to target
		return objectPreventExtensionsWithVM(vmInstance, []vm.Value{proxy.Target()})
	}

	// Per ECMAScript spec, primitives are returned unchanged
	if !obj.IsObject() {
		return obj, nil
	}

	// Handle arrays specially
	if obj.Type() == vm.TypeArray {
		arr := obj.AsArray()
		arr.SetExtensible(false)
		return obj, nil
	}

	// Mark the object as non-extensible
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		plainObj.SetExtensible(false)
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		dictObj.SetExtensible(false)
	}

	return obj, nil
}

func objectFreezeWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, nil
	}

	obj := args[0]

	// If not an object, return as-is (primitives are already immutable)
	if !obj.IsObject() {
		return obj, nil
	}

	// Handle arrays specially
	if obj.Type() == vm.TypeArray {
		arr := obj.AsArray()
		arr.SetExtensible(false)
		return obj, nil
	}

	// Freeze: make object non-extensible and all properties non-configurable and non-writable
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		plainObj.SetExtensible(false)
		// Mark all properties as non-configurable and non-writable
		for _, key := range plainObj.OwnKeys() {
			value, _, enumerable, _, ok := plainObj.GetOwnDescriptor(key)
			if ok {
				w := false
				c := false
				plainObj.DefineOwnProperty(key, value, &w, &enumerable, &c)
			}
		}
	}

	return obj, nil
}

func objectSealWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.Undefined, nil
	}

	obj := args[0]

	// If not an object, return as-is
	if !obj.IsObject() {
		return obj, nil
	}

	// Seal: make object non-extensible and all properties non-configurable (but leave writable as-is)
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		plainObj.SetExtensible(false)
		for _, key := range plainObj.OwnKeys() {
			value, writable, enumerable, _, ok := plainObj.GetOwnDescriptor(key)
			if ok {
				c := false
				plainObj.DefineOwnProperty(key, value, &writable, &enumerable, &c)
			}
		}
	}

	return obj, nil
}

func objectIsFrozenWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.BooleanValue(true), nil
	}

	obj := args[0]

	// Primitives are frozen
	if !obj.IsObject() {
		return vm.BooleanValue(true), nil
	}

	// Handle arrays specially
	if obj.Type() == vm.TypeArray {
		arr := obj.AsArray()
		return vm.BooleanValue(!arr.IsExtensible()), nil
	}

	// Check if extensible - frozen objects must not be extensible
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		if plainObj.IsExtensible() {
			return vm.BooleanValue(false), nil
		}
		// Check all properties are non-configurable and non-writable
		for _, key := range plainObj.OwnKeys() {
			_, writable, _, configurable, ok := plainObj.GetOwnDescriptor(key)
			if ok {
				if configurable || writable {
					return vm.BooleanValue(false), nil
				}
			}
		}
		return vm.BooleanValue(true), nil
	}

	return vm.BooleanValue(false), nil
}

func objectIsSealedWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.BooleanValue(true), nil
	}

	obj := args[0]

	// Primitives are sealed
	if !obj.IsObject() {
		return vm.BooleanValue(true), nil
	}

	// Check if extensible - sealed objects must not be extensible
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		if plainObj.IsExtensible() {
			return vm.BooleanValue(false), nil
		}
		// Check all properties are non-configurable
		for _, key := range plainObj.OwnKeys() {
			_, _, _, configurable, ok := plainObj.GetOwnDescriptor(key)
			if ok {
				if configurable {
					return vm.BooleanValue(false), nil
				}
			}
		}
		return vm.BooleanValue(true), nil
	}

	return vm.BooleanValue(false), nil
}

// ObjectGetOwnPropertyDescriptorForHarness exposes a minimal descriptor getter for the test262 harness
func ObjectGetOwnPropertyDescriptorForHarness(obj vm.Value, name vm.Value) (vm.Value, error) {
	return objectGetOwnPropertyDescriptorWithVM(nil, []vm.Value{obj, name})
}

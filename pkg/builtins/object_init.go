package builtins

import (
	"fmt"
	"math"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strconv"
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
			if plainObj := thisValue.AsPlainObject(); plainObj != nil {
				return vm.BooleanValue(plainObj.HasOwnByKey(key)), nil
			}
			if dictObj := thisValue.AsDictObject(); dictObj != nil {
				// DictObject has only string keys; symbols are not supported
				return vm.BooleanValue(false), nil
			}
			if arrObj := thisValue.AsArray(); arrObj != nil {
				// Arrays: symbol own keys generally none here
				return vm.BooleanValue(false), nil
			}
			return vm.BooleanValue(false), nil
		}
		propName := keyVal.ToString()

		// Check if this object has the property as own property
		if plainObj := thisValue.AsPlainObject(); plainObj != nil {
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
		}
		if dictObj := thisValue.AsDictObject(); dictObj != nil {
			_, hasOwn := dictObj.GetOwn(propName)
			return vm.BooleanValue(hasOwn), nil
		}
		if arrObj := thisValue.AsArray(); arrObj != nil {
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
		if po := thisValue.AsPlainObject(); po != nil {
			if _, _, en, _, ok := po.GetOwnDescriptor(propName); ok {
				return vm.BooleanValue(en), nil
			}
			return vm.BooleanValue(false), nil
		}
		if dict := thisValue.AsDictObject(); dict != nil {
			if _, _, en, _, ok := dict.GetOwnDescriptor(propName); ok {
				return vm.BooleanValue(en), nil
			}
			return vm.BooleanValue(false), nil
		}
		if arr := thisValue.AsArray(); arr != nil {
			if propName == "length" {
				return vm.BooleanValue(false), nil
			}
			if idx, err := strconv.Atoi(propName); err == nil && idx >= 0 && idx < arr.Length() {
				return vm.BooleanValue(true), nil
			}
		}
		return vm.BooleanValue(false), nil
	}))
	if v, ok := objectProto.GetOwn("propertyIsEnumerable"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("propertyIsEnumerable", v, &w, &e, &c)
	}

	objectProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()

		// Return appropriate string representation based on type
		switch thisValue.Type() {
		case vm.TypeNull:
			return vm.NewString("[object Null]"), nil
		case vm.TypeUndefined:
			return vm.NewString("[object Undefined]"), nil
		case vm.TypeBoolean:
			return vm.NewString("[object Boolean]"), nil
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return vm.NewString("[object Number]"), nil
		case vm.TypeString:
			return vm.NewString("[object String]"), nil
		case vm.TypeArray:
			return vm.NewString("[object Array]"), nil
		case vm.TypeFunction, vm.TypeNativeFunction, vm.TypeClosure:
			return vm.NewString("[object Function]"), nil
		case vm.TypeObject:
			// Check for wrapper objects with [[PrimitiveValue]]
			if plainObj := thisValue.AsPlainObject(); plainObj != nil {
				if primitiveVal, exists := plainObj.GetOwn("[[PrimitiveValue]]"); exists {
					switch primitiveVal.Type() {
					case vm.TypeBoolean:
						return vm.NewString("[object Boolean]"), nil
					case vm.TypeFloatNumber, vm.TypeIntegerNumber:
						return vm.NewString("[object Number]"), nil
					case vm.TypeString:
						return vm.NewString("[object String]"), nil
					}
				}
			}
			return vm.NewString("[object Object]"), nil
		default:
			return vm.NewString("[object Object]"), nil
		}
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
			case vm.TypeFunction, vm.TypeClosure:
				// Functions have FunctionPrototype as their prototype
				proto = vmInstance.FunctionPrototype
			case vm.TypeNativeFunctionWithProps:
				// NativeFunctionWithProps has its Properties' prototype
				if nfp := current.AsNativeFunctionWithProps(); nfp != nil {
					proto = nfp.Properties.GetPrototype()
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

	// Create Object constructor
	objectCtor := vm.NewNativeFunction(-1, true, "Object", func(args []vm.Value) (vm.Value, error) {
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
		ctorPropsObj.Properties.SetOwnNonEnumerable("values", vm.NewNativeFunction(1, false, "values", objectValuesImpl))
		ctorPropsObj.Properties.SetOwnNonEnumerable("entries", vm.NewNativeFunction(1, false, "entries", objectEntriesImpl))
		ctorPropsObj.Properties.SetOwnNonEnumerable("getOwnPropertyNames", vm.NewNativeFunction(1, false, "getOwnPropertyNames", objectGetOwnPropertyNamesImpl))
		ctorPropsObj.Properties.SetOwnNonEnumerable("getOwnPropertySymbols", vm.NewNativeFunction(1, false, "getOwnPropertySymbols", objectGetOwnPropertySymbolsImpl))
		// Reflect-like ownKeys: strings first, then symbols
		ctorPropsObj.Properties.SetOwnNonEnumerable("__ownKeys", vm.NewNativeFunction(1, false, "__ownKeys", reflectOwnKeysImpl))
		ctorPropsObj.Properties.SetOwnNonEnumerable("assign", vm.NewNativeFunction(1, true, "assign", func(args []vm.Value) (vm.Value, error) {
			return objectAssignWithVM(vmInstance, args)
		}))
		ctorPropsObj.Properties.SetOwnNonEnumerable("hasOwn", vm.NewNativeFunction(2, false, "hasOwn", objectHasOwnImpl))
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
			if len(args) < 2 {
				return vm.BooleanValue(false), nil
			}
			return vm.BooleanValue(sameValue(args[0], args[1])), nil
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

	// If properties descriptor is provided, define properties
	if len(args) >= 2 && !args[1].IsUndefined() {
		propertiesDesc := args[1]
		if !propertiesDesc.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("Properties must be an object")
		}

		// Iterate over own enumerable properties of the descriptor object
		plainObj := obj.AsPlainObject()
		if plainObj == nil {
			return vm.Undefined, vmInstance.NewTypeError("Created object is not a plain object")
		}

		descObj := propertiesDesc.AsPlainObject()
		if descObj != nil {
			for _, key := range descObj.OwnKeys() {
				propDesc, _, enumerable, _, ok := descObj.GetOwnDescriptor(key)
				if !ok || !enumerable {
					continue // Only process own enumerable properties
				}

				// propDesc should be an object containing property descriptor
				if !propDesc.IsObject() {
					continue
				}

				propDescObj := propDesc.AsPlainObject()
				if propDescObj == nil {
					continue
				}

				// Extract descriptor properties
				var value vm.Value
				var writable, enumFlag, configurable bool
				var hasValue, hasWritable, hasEnumerable, hasConfigurable bool

				// Check for 'value' property
				if v, ok := propDescObj.GetOwn("value"); ok {
					value = v
					hasValue = true
				}

				// Check for 'writable' property
				if v, ok := propDescObj.GetOwn("writable"); ok {
					writable = v.AsBoolean()
					hasWritable = true
				}

				// Check for 'enumerable' property
				if v, ok := propDescObj.GetOwn("enumerable"); ok {
					enumFlag = v.AsBoolean()
					hasEnumerable = true
				}

				// Check for 'configurable' property
				if v, ok := propDescObj.GetOwn("configurable"); ok {
					configurable = v.AsBoolean()
					hasConfigurable = true
				}

				// Apply defaults: if not specified, writable/enumerable/configurable default to false
				if !hasValue {
					value = vm.Undefined
				}

				// Use pointers for DefineOwnProperty
				var wPtr, ePtr, cPtr *bool
				if hasWritable {
					wPtr = &writable
				} else {
					// Default to false
					f := false
					wPtr = &f
				}
				if hasEnumerable {
					ePtr = &enumFlag
				} else {
					// Default to false
					f := false
					ePtr = &f
				}
				if hasConfigurable {
					cPtr = &configurable
				} else {
					// Default to false
					f := false
					cPtr = &f
				}

				plainObj.DefineOwnProperty(key, value, wPtr, ePtr, cPtr)
			}
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

	propertiesDesc := args[1]
	if !propertiesDesc.IsObject() {
		return vm.Undefined, vmInstance.NewTypeError("Properties must be an object")
	}

	// Get the plain object to define properties on
	plainObj := obj.AsPlainObject()
	if plainObj == nil {
		return vm.Undefined, vmInstance.NewTypeError("Cannot define properties on non-plain object")
	}

	// Iterate over own enumerable properties of the descriptor object
	descObj := propertiesDesc.AsPlainObject()
	if descObj != nil {
		for _, key := range descObj.OwnKeys() {
			propDesc, _, enumerable, _, ok := descObj.GetOwnDescriptor(key)
			if !ok || !enumerable {
				continue // Only process own enumerable properties
			}

			// propDesc should be an object containing property descriptor
			if !propDesc.IsObject() {
				continue
			}

			propDescObj := propDesc.AsPlainObject()
			if propDescObj == nil {
				continue
			}

			// Extract descriptor properties
			var value vm.Value
			var writable, enumFlag, configurable bool
			var hasValue, hasWritable, hasEnumerable, hasConfigurable bool

			// Check for 'value' property
			if v, ok := propDescObj.GetOwn("value"); ok {
				value = v
				hasValue = true
			}

			// Check for 'writable' property
			if v, ok := propDescObj.GetOwn("writable"); ok {
				writable = v.AsBoolean()
				hasWritable = true
			}

			// Check for 'enumerable' property
			if v, ok := propDescObj.GetOwn("enumerable"); ok {
				enumFlag = v.AsBoolean()
				hasEnumerable = true
			}

			// Check for 'configurable' property
			if v, ok := propDescObj.GetOwn("configurable"); ok {
				configurable = v.AsBoolean()
				hasConfigurable = true
			}

			// Apply defaults: if not specified, writable/enumerable/configurable default to false
			if !hasValue {
				value = vm.Undefined
			}

			// Use pointers for DefineOwnProperty
			var wPtr, ePtr, cPtr *bool
			if hasWritable {
				wPtr = &writable
			} else {
				// Default to false
				f := false
				wPtr = &f
			}
			if hasEnumerable {
				ePtr = &enumFlag
			} else {
				// Default to false
				f := false
				ePtr = &f
			}
			if hasConfigurable {
				cPtr = &configurable
			} else {
				// Default to false
				f := false
				cPtr = &f
			}

			plainObj.DefineOwnProperty(key, value, wPtr, ePtr, cPtr)
		}
	}

	return obj, nil
}

func objectKeysWithVM(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.NewArray(), nil
	}

	obj := args[0]
	if !obj.IsObject() {
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

	// Handle regular objects
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		for _, key := range plainObj.OwnKeys() {
			if _, _, en, _, ok := plainObj.GetOwnDescriptor(key); ok && en {
				keysArray.Append(vm.NewString(key))
			}
		}
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		for _, key := range dictObj.OwnKeys() {
			keysArray.Append(vm.NewString(key))
		}
	} else if arrObj := obj.AsArray(); arrObj != nil {
		for i := 0; i < arrObj.Length(); i++ {
			keysArray.Append(vm.NewString(strconv.Itoa(i)))
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
			return vm.Undefined, fmt.Errorf("Cannot get prototype of revoked Proxy")
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

	// First argument must be an object
	if obj.Type() != vm.TypeObject {
		return vm.Undefined, vmInstance.NewTypeError("Object.setPrototypeOf called on non-object")
	}

	// Set the prototype
	success := true
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		success = plainObj.SetPrototype(proto)
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		success = dictObj.SetPrototype(proto)
	}

	if !success {
		return vm.Undefined, vmInstance.NewTypeError("Cannot set prototype of non-extensible object")
	}

	// Return the object
	return obj, nil
}

func objectValuesImpl(args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.NewArray(), nil
	}

	obj := args[0]
	if !obj.IsObject() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.NewArray(), nil
	}

	values := vm.NewArray()
	valuesArray := values.AsArray()

	if plainObj := obj.AsPlainObject(); plainObj != nil {
		for _, key := range plainObj.OwnKeys() {
			if _, _, en, _, ok := plainObj.GetOwnDescriptor(key); ok && en {
				value, _ := plainObj.GetOwn(key)
				valuesArray.Append(value)
			}
		}
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		for _, key := range dictObj.OwnKeys() {
			value, _ := dictObj.GetOwn(key)
			valuesArray.Append(value)
		}
	} else if arrObj := obj.AsArray(); arrObj != nil {
		// For arrays, return the element values
		for i := 0; i < arrObj.Length(); i++ {
			valuesArray.Append(arrObj.Get(i))
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

func objectEntriesImpl(args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.NewArray(), nil
	}

	obj := args[0]
	if !obj.IsObject() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.NewArray(), nil
	}

	entries := vm.NewArray()
	entriesArray := entries.AsArray()

	if plainObj := obj.AsPlainObject(); plainObj != nil {
		for _, key := range plainObj.OwnKeys() {
			if _, _, en, _, ok := plainObj.GetOwnDescriptor(key); ok && en {
				value, _ := plainObj.GetOwn(key)
				entry := vm.NewArray()
				entry.AsArray().Append(vm.NewString(key))
				entry.AsArray().Append(value)
				entriesArray.Append(entry)
			}
		}
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		for _, key := range dictObj.OwnKeys() {
			value, _ := dictObj.GetOwn(key)
			entry := vm.NewArray()
			entry.AsArray().Append(vm.NewString(key))
			entry.AsArray().Append(value)
			entriesArray.Append(entry)
		}
	} else if arrObj := obj.AsArray(); arrObj != nil {
		// For arrays, return [index, value] pairs
		for i := 0; i < arrObj.Length(); i++ {
			entry := vm.NewArray()
			entry.AsArray().Append(vm.NewString(strconv.Itoa(i)))
			entry.AsArray().Append(arrObj.Get(i))
			entriesArray.Append(entry)
		}
	}

	return entries, nil
}

func objectGetOwnPropertyNamesImpl(args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.NewArray(), nil
	}
	obj := args[0]
	if !obj.IsObject() {
		return vm.NewArray(), nil
	}
	arr := vm.NewArray()
	arrObj := arr.AsArray()
	if po := obj.AsPlainObject(); po != nil {
		for _, k := range po.OwnKeys() {
			arrObj.Append(vm.NewString(k))
		}
	} else if d := obj.AsDictObject(); d != nil {
		for _, k := range d.OwnKeys() {
			arrObj.Append(vm.NewString(k))
		}
	} else if a := obj.AsArray(); a != nil {
		for i := 0; i < a.Length(); i++ {
			arrObj.Append(vm.NewString(strconv.Itoa(i)))
		}
		arrObj.Append(vm.NewString("length"))
	}
	return arr, nil
}

func objectGetOwnPropertySymbolsImpl(args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		return vm.NewArray(), nil
	}
	obj := args[0]
	if !obj.IsObject() {
		return vm.NewArray(), nil
	}
	arr := vm.NewArray()
	arrObj := arr.AsArray()
	if po := obj.AsPlainObject(); po != nil {
		for _, s := range po.OwnSymbolKeys() {
			arrObj.Append(s)
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
		return vm.Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
	}

	// First argument is the target
	target := args[0]

	// Convert primitives to objects (except null/undefined which throw)
	if target.Type() == vm.TypeNull || target.Type() == vm.TypeUndefined {
		return vm.Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
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

func objectHasOwnImpl(args []vm.Value) (vm.Value, error) {
	if len(args) < 2 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.BooleanValue(false), nil
	}

	obj := args[0]
	keyVal := args[1]

	// Check if object has the property as own property
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		if keyVal.Type() == vm.TypeSymbol {
			return vm.BooleanValue(plainObj.HasOwnByKey(vm.NewSymbolKey(keyVal))), nil
		}
		_, hasOwn := plainObj.GetOwn(keyVal.ToString())
		return vm.BooleanValue(hasOwn), nil
	}
	if dictObj := obj.AsDictObject(); dictObj != nil {
		if keyVal.Type() == vm.TypeSymbol {
			return vm.BooleanValue(false), nil
		}
		_, hasOwn := dictObj.GetOwn(keyVal.ToString())
		return vm.BooleanValue(hasOwn), nil
	}
	if arrObj := obj.AsArray(); arrObj != nil {
		if keyVal.Type() == vm.TypeSymbol {
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
	// Property key: support symbols natively
	var keyIsSymbol bool
	var propName string
	var propSym vm.Value
	if args[1].Type() == vm.TypeSymbol {
		keyIsSymbol = true
		propSym = args[1]
	} else {
		propName = args[1].ToString()
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
		obj.Type() == vm.TypeNativeFunctionWithProps
	if !isObjectLike {
		return vm.Undefined, vmInstance.NewTypeError("Object.defineProperty called on non-object")
	}

	// Parse descriptor object fields: value, writable, enumerable, configurable, get, set
	var value vm.Value = vm.Undefined
	var writablePtr, enumerablePtr, configurablePtr *bool
	var getter vm.Value = vm.Undefined
	var setter vm.Value = vm.Undefined
	hasGetter := false
	hasSetter := false
	if descObj := descriptor.AsPlainObject(); descObj != nil {
		if val, exists := descObj.GetOwn("value"); exists {
			value = val
		}
		if w, exists := descObj.GetOwn("writable"); exists {
			b := w.IsTruthy()
			writablePtr = &b
		}
		if e, exists := descObj.GetOwn("enumerable"); exists {
			b := e.IsTruthy()
			enumerablePtr = &b
		}
		if c, exists := descObj.GetOwn("configurable"); exists {
			b := c.IsTruthy()
			configurablePtr = &b
		}
		if g, exists := descObj.GetOwn("get"); exists {
			hasGetter = true
			getter = g
		}
		if s, exists := descObj.GetOwn("set"); exists {
			hasSetter = true
			setter = s
		}
	} else if descObj := descriptor.AsDictObject(); descObj != nil {
		if val, exists := descObj.GetOwn("value"); exists {
			value = val
		}
		if w, exists := descObj.GetOwn("writable"); exists {
			b := w.IsTruthy()
			writablePtr = &b
		}
		if e, exists := descObj.GetOwn("enumerable"); exists {
			b := e.IsTruthy()
			enumerablePtr = &b
		}
		if c, exists := descObj.GetOwn("configurable"); exists {
			b := c.IsTruthy()
			configurablePtr = &b
		}
		if g, exists := descObj.GetOwn("get"); exists {
			hasGetter = true
			getter = g
		}
		if s, exists := descObj.GetOwn("set"); exists {
			hasSetter = true
			setter = s
		}
	} else {
		// Non-object descriptor treated as { value: descriptor }
		value = descriptor
	}

	// If accessor fields present with data fields, throw TypeError
	if (hasGetter || hasSetter) && (value.Type() != vm.TypeUndefined || writablePtr != nil) {
		// In absence of a direct VM reference here, mimic failure by returning undefined;
		// harness verifyProperty will report descriptor mismatch. We will revisit once we thread VM here.
		return vm.Undefined, nil
	}

	// Explicitly default missing attributes to false for data descriptors
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

	// Define the property with attributes (on plain objects only for now)
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		// Enforce non-configurable rules when updating existing property
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
			if v, w, e, c, ok := fn.Properties.GetOwnDescriptor(propName); ok {
				descriptor := vm.NewObject(vm.Undefined).AsPlainObject()
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
		fn := closure.Fn
		// Check custom properties first
		if fn.Properties != nil {
			if v, w, e, c, ok := fn.Properties.GetOwnDescriptor(propName); ok {
				descriptor := vm.NewObject(vm.Undefined).AsPlainObject()
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
			if v, w, e, c, ok := nfp.Properties.GetOwnDescriptor(propName); ok {
				descriptor := vm.NewObject(vm.Undefined).AsPlainObject()
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
		// For arrays, check if it's a valid index or 'length'
		if propName == "length" {
			value = vm.NumberValue(float64(arrObj.Length()))
			// length is non-enumerable, non-configurable, writable per JS spec for Array.length
			descriptor := vm.NewObject(vm.Undefined).AsPlainObject()
			descriptor.SetOwn("value", value)
			descriptor.SetOwn("writable", vm.BooleanValue(true))
			descriptor.SetOwn("enumerable", vm.BooleanValue(false))
			descriptor.SetOwn("configurable", vm.BooleanValue(false))
			return vm.NewValueFromPlainObject(descriptor), nil
		} else if index, err := strconv.Atoi(propName); err == nil && index >= 0 && index < arrObj.Length() {
			value = arrObj.Get(index)
			descriptor := vm.NewObject(vm.Undefined).AsPlainObject()
			descriptor.SetOwn("value", value)
			descriptor.SetOwn("writable", vm.BooleanValue(true))
			descriptor.SetOwn("enumerable", vm.BooleanValue(true))
			descriptor.SetOwn("configurable", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// Fall through for non-index properties on arrays (methods, custom props)
	}

	if plainObj := obj.AsPlainObject(); plainObj != nil {
		if g, s, e, c, ok := func() (vm.Value, vm.Value, bool, bool, bool) {
			if keyIsSymbol {
				return plainObj.GetOwnAccessorByKey(vm.NewSymbolKey(propSym))
			}
			return plainObj.GetOwnAccessor(propName)
		}(); ok {
			// Accessor descriptor
			descriptor := vm.NewObject(vm.Undefined).AsPlainObject()
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
			descriptor := vm.NewObject(vm.Undefined).AsPlainObject()
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
				descriptor := vm.NewObject(vm.Undefined).AsPlainObject()
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
		// not found
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		if v, w, e, c, ok := dictObj.GetOwnDescriptor(propName); ok {
			descriptor := vm.NewObject(vm.Undefined).AsPlainObject()
			descriptor.SetOwn("value", v)
			descriptor.SetOwn("writable", vm.BooleanValue(w))
			descriptor.SetOwn("enumerable", vm.BooleanValue(e))
			descriptor.SetOwn("configurable", vm.BooleanValue(c))
			return vm.NewValueFromPlainObject(descriptor), nil
		}
		// not found
	}

	return vm.Undefined, nil
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

	// Check if object is extensible
	if obj.IsObject() {
		if plainObj := obj.AsPlainObject(); plainObj != nil {
			return vm.BooleanValue(plainObj.IsExtensible()), nil
		}
		// Other object types (DictObject, etc.) are extensible by default for now
		return vm.BooleanValue(true), nil
	}

	// Functions and closures are objects and extensible by default per ECMAScript
	if obj.Type() == vm.TypeFunction || obj.Type() == vm.TypeClosure {
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

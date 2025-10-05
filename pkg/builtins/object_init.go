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
		WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("isPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean))

	// Create Object constructor type using fluent API
	objectCtorType := types.NewObjectType().
		// Constructor is callable with optional parameter
		WithSimpleCallSignature([]types.Type{}, types.Any).
		WithSimpleCallSignature([]types.Type{types.Any}, types.Any).
		// Static methods
		WithProperty("create", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
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
		WithProperty("getOwnPropertyDescriptor", types.NewSimpleFunction([]types.Type{types.Any, keyStringOrSymbol}, types.Any)).
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
	objectProto.SetOwn("hasOwnProperty", vm.NewNativeFunction(1, false, "hasOwnProperty", func(args []vm.Value) (vm.Value, error) {
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
	objectProto.SetOwn("propertyIsEnumerable", vm.NewNativeFunction(1, false, "propertyIsEnumerable", func(args []vm.Value) (vm.Value, error) {
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

	objectProto.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
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
		default:
			return vm.NewString("[object Object]"), nil
		}
	}))
	if v, ok := objectProto.GetOwn("toString"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("toString", v, &w, &e, &c)
	}

	objectProto.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.GetThis(), nil // Return this
	}))
	if v, ok := objectProto.GetOwn("valueOf"); ok {
		w, e, c := true, false, true
		objectProto.DefineOwnProperty("valueOf", v, &w, &e, &c)
	}

	objectProto.SetOwn("isPrototypeOf", vm.NewNativeFunction(1, false, "isPrototypeOf", func(args []vm.Value) (vm.Value, error) {
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
			if plainObj := current.AsPlainObject(); plainObj != nil {
				proto = plainObj.GetPrototype()
			} else if dictObj := current.AsDictObject(); dictObj != nil {
				proto = dictObj.GetPrototype()
			} else {
				break // No prototype
			}

			// If we reached null, we're done
			if proto.Type() == vm.TypeNull {
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
			wrapper.SetOwn("[[PrimitiveValue]]", arg)
			// Add valueOf method that returns the primitive
			wrapper.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
				return arg, nil
			}))
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			// Create Number wrapper object
			wrapper := vm.NewObject(vmInstance.NumberPrototype).AsPlainObject()
			wrapper.SetOwn("[[PrimitiveValue]]", arg)
			wrapper.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
				return arg, nil
			}))
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeString:
			// Create String wrapper object
			wrapper := vm.NewObject(vmInstance.StringPrototype).AsPlainObject()
			wrapper.SetOwn("[[PrimitiveValue]]", arg)
			wrapper.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
				return arg, nil
			}))
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeBoolean:
			// Create Boolean wrapper object
			wrapper := vm.NewObject(vmInstance.BooleanPrototype).AsPlainObject()
			wrapper.SetOwn("[[PrimitiveValue]]", arg)
			wrapper.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
				return arg, nil
			}))
			return vm.NewValueFromPlainObject(wrapper), nil

		case vm.TypeSymbol:
			// Create Symbol wrapper object
			wrapper := vm.NewObject(vmInstance.SymbolPrototype).AsPlainObject()
			wrapper.SetOwn("[[PrimitiveValue]]", arg)
			wrapper.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
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
		ctorWithProps := vm.NewNativeFunctionWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(objectProto))
		if v, ok := ctorPropsObj.Properties.GetOwn("prototype"); ok {
			w, e, c := false, false, false
			ctorPropsObj.Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
		}

		// Add static methods
		ctorPropsObj.Properties.SetOwn("create", vm.NewNativeFunction(1, false, "create", objectCreateImpl))
		ctorPropsObj.Properties.SetOwn("keys", vm.NewNativeFunction(1, false, "keys", objectKeysImpl))
		ctorPropsObj.Properties.SetOwn("values", vm.NewNativeFunction(1, false, "values", objectValuesImpl))
		ctorPropsObj.Properties.SetOwn("entries", vm.NewNativeFunction(1, false, "entries", objectEntriesImpl))
		ctorPropsObj.Properties.SetOwn("getOwnPropertyNames", vm.NewNativeFunction(1, false, "getOwnPropertyNames", objectGetOwnPropertyNamesImpl))
		ctorPropsObj.Properties.SetOwn("getOwnPropertySymbols", vm.NewNativeFunction(1, false, "getOwnPropertySymbols", objectGetOwnPropertySymbolsImpl))
		// Reflect-like ownKeys: strings first, then symbols
		ctorPropsObj.Properties.SetOwn("__ownKeys", vm.NewNativeFunction(1, false, "__ownKeys", reflectOwnKeysImpl))
		ctorPropsObj.Properties.SetOwn("assign", vm.NewNativeFunction(1, true, "assign", objectAssignImpl))
		ctorPropsObj.Properties.SetOwn("hasOwn", vm.NewNativeFunction(2, false, "hasOwn", objectHasOwnImpl))
		ctorPropsObj.Properties.SetOwn("fromEntries", vm.NewNativeFunction(1, false, "fromEntries", objectFromEntriesImpl))
		ctorPropsObj.Properties.SetOwn("getPrototypeOf", vm.NewNativeFunction(1, false, "getPrototypeOf", objectGetPrototypeOfImpl))
		ctorPropsObj.Properties.SetOwn("setPrototypeOf", vm.NewNativeFunction(2, false, "setPrototypeOf", objectSetPrototypeOfImpl))
		// defineProperty delegates to the full implementation with symbol/accessor support
		ctorPropsObj.Properties.SetOwn("defineProperty", vm.NewNativeFunction(3, false, "defineProperty", objectDefinePropertyImpl))
		ctorPropsObj.Properties.SetOwn("getOwnPropertyDescriptor", vm.NewNativeFunction(2, false, "getOwnPropertyDescriptor", objectGetOwnPropertyDescriptorImpl))
		// Object.is
		ctorPropsObj.Properties.SetOwn("is", vm.NewNativeFunction(2, false, "is", func(args []vm.Value) (vm.Value, error) {
			if len(args) < 2 {
				return vm.BooleanValue(false), nil
			}
			return vm.BooleanValue(sameValue(args[0], args[1])), nil
		}))

		objectCtor = ctorWithProps
	}

	// Set constructor property on prototype
	objectProto.SetOwn("constructor", objectCtor)
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

func objectCreateImpl(args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined, nil
	}

	proto := args[0]

	// Check if proto is null or an object
	if proto.Type() != vm.TypeNull && proto.Type() != vm.TypeObject {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined, nil
	}

	// Create a new object with the specified prototype
	if proto.Type() == vm.TypeNull {
		// For null prototype, create object and set prototype to null
		obj := vm.NewObject(vm.Null)
		if plainObj := obj.AsPlainObject(); plainObj != nil {
			plainObj.SetPrototype(vm.Null)
		}
		return obj, nil
	} else {
		// For object prototype, NewObject handles it correctly
		return vm.NewObject(proto), nil
	}
}

func objectKeysImpl(args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.NewArray(), nil
	}

	obj := args[0]
	if !obj.IsObject() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.NewArray(), nil
	}

	keys := vm.NewArray()
	keysArray := keys.AsArray()

	if plainObj := obj.AsPlainObject(); plainObj != nil {
		for _, key := range plainObj.OwnKeys() {
			if _, _, en, _, ok := plainObj.GetOwnDescriptor(key); ok && en {
				keysArray.Append(vm.NewString(key))
			}
		}
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		for _, key := range dictObj.OwnKeys() {
			// DictObject defaults to enumerable
			keysArray.Append(vm.NewString(key))
		}
	} else if arrObj := obj.AsArray(); arrObj != nil {
		// Arrays: own enumerable string keys are indices present
		for i := 0; i < arrObj.Length(); i++ {
			keysArray.Append(vm.NewString(strconv.Itoa(i)))
		}
	}

	return keys, nil
}

// (Removed duplicate alternate implementations of objectValuesImpl and objectEntriesImpl)

func objectGetPrototypeOfImpl(args []vm.Value) (vm.Value, error) {
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
		// For arrays, return Array.prototype if available
		// This will be set up when ArrayInitializer runs
		return vm.Null, nil // TODO: Return proper Array.prototype
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
		// For proxies, delegate to the target's prototype
		// Note: getPrototypeOf trap should be called from VM's [[GetPrototypeOf]]
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.Undefined, fmt.Errorf("Cannot get prototype of revoked Proxy")
		}
		// Delegate to target
		return objectGetPrototypeOfImpl([]vm.Value{proxy.Target()})
	default:
		// For primitive values, return null
		return vm.Null, nil
	}
}

func objectSetPrototypeOfImpl(args []vm.Value) (vm.Value, error) {
	if len(args) < 2 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined, nil
	}

	obj := args[0]
	proto := args[1]

	// First argument must be an object
	if obj.Type() != vm.TypeObject {
		// TODO: Throw TypeError when error objects are implemented
		return obj, nil // Return the object unchanged as per spec
	}

	// Second argument must be an object or null
	if proto.Type() != vm.TypeNull && proto.Type() != vm.TypeObject {
		// TODO: Throw TypeError when error objects are implemented
		return obj, nil // Return the object unchanged
	}

	// Set the prototype
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		plainObj.SetPrototype(proto)
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		dictObj.SetPrototype(proto)
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

func objectAssignImpl(args []vm.Value) (vm.Value, error) {
	if len(args) == 0 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined, nil
	}

	// First argument is the target
	target := args[0]

	// Convert primitives to objects (except null/undefined)
	if target.Type() == vm.TypeNull || target.Type() == vm.TypeUndefined {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined, nil
	}

	// If target is not an object, box it
	if !target.IsObject() {
		// TODO: Implement primitive boxing
		return target, nil
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
					targetPlain.SetOwn(key, value)
				} else if targetDict := target.AsDictObject(); targetDict != nil {
					targetDict.SetOwn(key, value)
				}
			}
		} else if dictObj := source.AsDictObject(); dictObj != nil {
			for _, key := range dictObj.OwnKeys() {
				value, _ := dictObj.GetOwn(key)
				// Set on target
				if targetPlain := target.AsPlainObject(); targetPlain != nil {
					targetPlain.SetOwn(key, value)
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
					targetPlain.SetOwn(key, value)
				} else if targetDict := target.AsDictObject(); targetDict != nil {
					targetDict.SetOwn(key, value)
				}
			}
			// Also copy length property
			if targetPlain := target.AsPlainObject(); targetPlain != nil {
				targetPlain.SetOwn("length", vm.NumberValue(float64(arrObj.Length())))
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
				resultObj.SetOwn(key, value)
			}
		}
	}
	// TODO: Support other iterables when iterator protocol is implemented

	return result, nil
}

func objectDefinePropertyImpl(args []vm.Value) (vm.Value, error) {
	if len(args) < 3 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined, nil
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

	// First argument must be an object
	if !obj.IsObject() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined, nil
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
	}

	return obj, nil
}

func objectGetOwnPropertyDescriptorImpl(args []vm.Value) (vm.Value, error) {
	if len(args) < 2 {
		// TODO: Throw TypeError when error objects are implemented
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

	// First argument must be object-like
	if !(obj.IsObject() || obj.AsArray() != nil) {
		return vm.Undefined, nil
	}

	// Check if the property exists
	var value vm.Value

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
	} else if arrObj := obj.AsArray(); arrObj != nil {
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
	}

	return vm.Undefined, nil
}

// ObjectGetOwnPropertyDescriptorForHarness exposes a minimal descriptor getter for the test262 harness
func ObjectGetOwnPropertyDescriptorForHarness(obj vm.Value, name vm.Value) (vm.Value, error) {
	return objectGetOwnPropertyDescriptorImpl([]vm.Value{obj, name})
}

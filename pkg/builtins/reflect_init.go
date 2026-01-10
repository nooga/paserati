package builtins

import (
	"strconv"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// isConstructor checks if a value can be used as a constructor (called with 'new')
func isConstructor(v vm.Value) bool {
	switch v.Type() {
	case vm.TypeFunction:
		fn := vm.AsFunction(v)
		// Arrow functions and async (non-generator) functions are not constructors
		if fn.IsArrowFunction || (fn.IsAsync && !fn.IsGenerator) {
			return false
		}
		return true
	case vm.TypeClosure:
		cl := vm.AsClosure(v)
		if cl.Fn.IsArrowFunction || (cl.Fn.IsAsync && !cl.Fn.IsGenerator) {
			return false
		}
		return true
	case vm.TypeNativeFunction:
		nf := vm.AsNativeFunction(v)
		return nf.IsConstructor
	case vm.TypeNativeFunctionWithProps:
		nfp := v.AsNativeFunctionWithProps()
		return nfp.IsConstructor
	case vm.TypeBoundFunction:
		// Bound functions inherit constructor status from original
		bf := v.AsBoundFunction()
		return isConstructor(bf.OriginalFunction)
	default:
		return false
	}
}

type ReflectInitializer struct{}

func (r *ReflectInitializer) Name() string  { return "Reflect" }
func (r *ReflectInitializer) Priority() int { return 104 } // After Console (102), Symbol must be initialized first

func (r *ReflectInitializer) InitTypes(ctx *TypeContext) error {
	// Property key type: string | symbol
	keyType := types.NewUnionType(types.String, types.Symbol)

	// Create Reflect object type with all 13 methods
	reflectType := types.NewObjectType().
		// Property operations
		WithProperty("get", types.NewSimpleFunction([]types.Type{types.Any, keyType}, types.Any)).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, keyType, types.Any}, types.Boolean)).
		WithProperty("has", types.NewSimpleFunction([]types.Type{types.Any, keyType}, types.Boolean)).
		WithProperty("deleteProperty", types.NewSimpleFunction([]types.Type{types.Any, keyType}, types.Boolean)).
		// Prototype operations
		WithProperty("getPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("setPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Boolean)).
		// Descriptor operations
		WithProperty("defineProperty", types.NewSimpleFunction([]types.Type{types.Any, keyType, types.Any}, types.Boolean)).
		WithProperty("getOwnPropertyDescriptor", types.NewSimpleFunction([]types.Type{types.Any, keyType}, types.Any)).
		// Key operations
		WithProperty("ownKeys", types.NewSimpleFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: types.Any})).
		// Extensibility operations
		WithProperty("isExtensible", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("preventExtensions", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		// Function operations
		WithProperty("apply", types.NewSimpleFunction([]types.Type{types.Any, types.Any, types.Any}, types.Any)).
		WithProperty("construct", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Any))

	return ctx.DefineGlobal("Reflect", reflectType)
}

func (r *ReflectInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Reflect object with Object.prototype as its prototype (ECMAScript spec)
	reflectObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Set @@toStringTag to "Reflect" so Object.prototype.toString.call(Reflect) returns "[object Reflect]"
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal := false
		reflectObj.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Reflect"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&falseVal, // configurable: false (per ECMAScript spec 28.1)
		)
	}

	// Reflect.get(target, propertyKey [, receiver])
	reflectObj.SetOwnNonEnumerable("get", vm.NewNativeFunction(2, false, "get", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.get requires at least 2 arguments")
		}
		target := args[0]
		propKey := args[1].ToString()

		if !target.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.get called on non-object")
		}

		// Perform simple property get
		switch target.Type() {
		case vm.TypeObject:
			if val, ok := target.AsPlainObject().Get(propKey); ok {
				return val, nil
			}
		case vm.TypeDictObject:
			if val, ok := target.AsDictObject().Get(propKey); ok {
				return val, nil
			}
		case vm.TypeArray:
			arr := target.AsArray()
			if propKey == "length" {
				return vm.Number(float64(arr.Length())), nil
			}
			// Try numeric index using strconv
			if idx, err := strconv.Atoi(propKey); err == nil && idx >= 0 && idx < arr.Length() {
				return arr.Get(idx), nil
			}
		}

		return vm.Undefined, nil
	}))

	// Reflect.set(target, propertyKey, value [, receiver])
	reflectObj.SetOwnNonEnumerable("set", vm.NewNativeFunction(3, false, "set", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.set requires at least 3 arguments")
		}
		target := args[0]
		propKey := args[1].ToString()
		value := args[2]

		if !target.IsObject() {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.set called on non-object")
		}

		// Perform simple property set
		switch target.Type() {
		case vm.TypeObject:
			target.AsPlainObject().SetOwnNonEnumerable(propKey, value)
			return vm.BooleanValue(true), nil
		case vm.TypeDictObject:
			target.AsDictObject().SetOwn(propKey, value)
			return vm.BooleanValue(true), nil
		case vm.TypeArray:
			arr := target.AsArray()
			if propKey == "length" {
				// Setting length is complex, skip for now
				return vm.BooleanValue(true), nil
			}
			if idx, err := strconv.Atoi(propKey); err == nil && idx >= 0 {
				arr.Set(idx, value)
				return vm.BooleanValue(true), nil
			}
		}

		return vm.BooleanValue(false), nil
	}))

	// Reflect.has(target, propertyKey)
	reflectObj.SetOwnNonEnumerable("has", vm.NewNativeFunction(2, false, "has", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.has requires 2 arguments")
		}
		target := args[0]
		propKey := args[1].ToString()

		if !target.IsObject() {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.has called on non-object")
		}

		// Use the 'in' operator logic
		hasProperty := false
		switch target.Type() {
		case vm.TypeObject:
			hasProperty = target.AsPlainObject().Has(propKey)
		case vm.TypeDictObject:
			hasProperty = target.AsDictObject().Has(propKey)
		case vm.TypeArray:
			arr := target.AsArray()
			if propKey == "length" {
				hasProperty = true
			} else if idx, err := strconv.Atoi(propKey); err == nil && idx >= 0 && idx < arr.Length() {
				hasProperty = true
			}
		}

		return vm.BooleanValue(hasProperty), nil
	}))

	// Reflect.deleteProperty(target, propertyKey)
	reflectObj.SetOwnNonEnumerable("deleteProperty", vm.NewNativeFunction(2, false, "deleteProperty", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.deleteProperty requires 2 arguments")
		}
		target := args[0]

		if !target.IsObject() {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.deleteProperty called on non-object")
		}

		// Delete property (simplified - just returns true)
		// A full implementation would actually remove the property
		// For now we don't have a Delete method on objects
		return vm.BooleanValue(true), nil
	}))

	// Reflect.getPrototypeOf(target) - reuse Object.getPrototypeOf
	reflectObj.SetOwnNonEnumerable("getPrototypeOf", vm.NewNativeFunction(1, false, "getPrototypeOf", func(args []vm.Value) (vm.Value, error) {
		return objectGetPrototypeOfWithVM(vmInstance, args)
	}))

	// Reflect.setPrototypeOf(target, prototype)
	reflectObj.SetOwnNonEnumerable("setPrototypeOf", vm.NewNativeFunction(2, false, "setPrototypeOf", func(args []vm.Value) (vm.Value, error) {
		result, err := objectSetPrototypeOfWithVM(vmInstance, args)
		if err != nil {
			return vm.BooleanValue(false), err
		}
		// Object.setPrototypeOf returns the object on success
		return vm.BooleanValue(result.Type() != vm.TypeUndefined), nil
	}))

	// Reflect.defineProperty(target, propertyKey, attributes)
	reflectObj.SetOwnNonEnumerable("defineProperty", vm.NewNativeFunction(3, false, "defineProperty", func(args []vm.Value) (vm.Value, error) {
		result, err := objectDefinePropertyWithVM(vmInstance, args)
		if err != nil {
			return vm.BooleanValue(false), err
		}
		// defineProperty returns the object on success
		return vm.BooleanValue(result.Type() != vm.TypeUndefined), nil
	}))

	// Reflect.getOwnPropertyDescriptor(target, propertyKey)
	reflectObj.SetOwnNonEnumerable("getOwnPropertyDescriptor", vm.NewNativeFunction(2, false, "getOwnPropertyDescriptor", func(args []vm.Value) (vm.Value, error) {
		return objectGetOwnPropertyDescriptorWithVM(vmInstance, args)
	}))

	// Reflect.ownKeys(target) - returns array of all own property keys
	reflectObj.SetOwnNonEnumerable("ownKeys", vm.NewNativeFunction(1, false, "ownKeys", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.ownKeys requires 1 argument")
		}
		target := args[0]

		if !target.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.ownKeys called on non-object")
		}

		// Get all own keys (including non-enumerable, unlike Object.keys)
		keysArray := vm.NewArray()
		arr := keysArray.AsArray()

		switch target.Type() {
		case vm.TypeObject:
			obj := target.AsPlainObject()
			// 1. String keys (all, including non-enumerable)
			for _, key := range obj.OwnPropertyNames() {
				arr.Append(vm.NewString(key))
			}
			// 2. Symbol keys
			for _, sym := range obj.OwnSymbolKeys() {
				arr.Append(sym)
			}
		case vm.TypeDictObject:
			// DictObject only supports string keys for now
			for _, key := range target.AsDictObject().OwnPropertyNames() {
				arr.Append(vm.NewString(key))
			}
		case vm.TypeArray:
			arrayObj := target.AsArray()
			// Add numeric indices
			for i := 0; i < arrayObj.Length(); i++ {
				arr.Append(vm.NewString(strconv.Itoa(i)))
			}
			// Add "length"
			arr.Append(vm.NewString("length"))
		case vm.TypeProxy:
			// For proxies, this should invoke the ownKeys trap
			// For now, delegate to Object.getOwnPropertyNames + getOwnPropertySymbols
			// This is a simplification
			if objCtor, ok := vmInstance.GetGlobal("Object"); ok {
				if nfp := objCtor.AsNativeFunctionWithProps(); nfp != nil {
					if f, ok := nfp.Properties.GetOwn("getOwnPropertyNames"); ok {
						if names, err := vmInstance.Call(f, vm.Undefined, []vm.Value{target}); err == nil {
							if namesArr := names.AsArray(); namesArr != nil {
								for i := 0; i < namesArr.Length(); i++ {
									arr.Append(namesArr.Get(i))
								}
							}
						}
					}
				}
			}
		}

		return keysArray, nil
	}))

	// Reflect.isExtensible(target)
	reflectObj.SetOwnNonEnumerable("isExtensible", vm.NewNativeFunction(1, false, "isExtensible", func(args []vm.Value) (vm.Value, error) {
		return objectIsExtensibleWithVM(vmInstance, args)
	}))

	// Reflect.preventExtensions(target)
	reflectObj.SetOwnNonEnumerable("preventExtensions", vm.NewNativeFunction(1, false, "preventExtensions", func(args []vm.Value) (vm.Value, error) {
		result, err := objectPreventExtensionsWithVM(vmInstance, args)
		if err != nil {
			return vm.BooleanValue(false), err
		}
		return vm.BooleanValue(result.Type() != vm.TypeUndefined), nil
	}))

	// Reflect.apply(target, thisArgument, argumentsList)
	reflectObj.SetOwnNonEnumerable("apply", vm.NewNativeFunction(3, false, "apply", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.apply requires 3 arguments")
		}
		target := args[0]
		thisArg := args[1]
		argsList := args[2]

		if !target.IsFunction() {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.apply: target is not a function")
		}

		// Convert argsList to array of values
		var callArgs []vm.Value
		if argsList.Type() == vm.TypeArray {
			arr := argsList.AsArray()
			callArgs = make([]vm.Value, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				callArgs[i] = arr.Get(i)
			}
		} else {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.apply: argumentsList must be an array")
		}

		// Call the function
		return vmInstance.Call(target, thisArg, callArgs)
	}))

	// Reflect.construct(target, argumentsList [, newTarget])
	reflectObj.SetOwnNonEnumerable("construct", vm.NewNativeFunction(2, false, "construct", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct requires at least 2 arguments")
		}
		target := args[0]
		argsList := args[1]

		// newTarget defaults to target
		newTarget := target
		if len(args) >= 3 {
			newTarget = args[2]
		}

		if !target.IsFunction() {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct: target is not a constructor")
		}

		// Check if target is actually a constructor (not arrow function, async, etc.)
		if !isConstructor(target) {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct: target is not a constructor")
		}

		// Check if newTarget is a constructor (per ECMAScript spec)
		if !isConstructor(newTarget) {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct: newTarget is not a constructor")
		}

		// Convert argsList to array of values
		var constructArgs []vm.Value
		if argsList.Type() == vm.TypeArray {
			arr := argsList.AsArray()
			constructArgs = make([]vm.Value, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				constructArgs[i] = arr.Get(i)
			}
		} else {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct: argumentsList must be an array")
		}

		// Call constructor with 'new' semantics (simplified)
		// We don't have a direct Construct method, so just return a simple object for now
		// A full implementation would invoke the constructor properly
		instance := vm.NewObject(vm.Undefined)
		result, err := vmInstance.Call(target, instance, constructArgs)
		if err != nil {
			return vm.Undefined, err
		}
		// If constructor returned an object, use it; otherwise use instance
		if result.IsObject() {
			return result, nil
		}
		return instance, nil
	}))

	// Define Reflect globally
	return ctx.DefineGlobal("Reflect", vm.NewValueFromPlainObject(reflectObj))
}

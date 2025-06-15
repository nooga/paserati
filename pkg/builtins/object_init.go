package builtins

import (
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
	objectProtoType := types.NewObjectType().
		WithProperty("hasOwnProperty", types.NewSimpleFunction([]types.Type{types.String}, types.Boolean)).
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
		WithProperty("getPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("setPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Any)).
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
	objectProto.SetOwn("hasOwnProperty", vm.NewNativeFunction(1, false, "hasOwnProperty", func(args []vm.Value) vm.Value {
		if len(args) < 1 {
			return vm.BooleanValue(false)
		}
		thisValue := vmInstance.GetThis()
		propName := args[0].ToString()

		// Check if this object has the property as own property
		if plainObj := thisValue.AsPlainObject(); plainObj != nil {
			_, hasOwn := plainObj.GetOwn(propName)
			return vm.BooleanValue(hasOwn)
		}
		if dictObj := thisValue.AsDictObject(); dictObj != nil {
			_, hasOwn := dictObj.GetOwn(propName)
			return vm.BooleanValue(hasOwn)
		}
		if arrObj := thisValue.AsArray(); arrObj != nil {
			// For arrays, check if it's a valid index or 'length'
			if propName == "length" {
				return vm.BooleanValue(true)
			}
			// Check numeric indices
			if index, err := strconv.Atoi(propName); err == nil {
				return vm.BooleanValue(index >= 0 && index < arrObj.Length())
			}
		}
		return vm.BooleanValue(false)
	}))

	objectProto.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) vm.Value {
		thisValue := vmInstance.GetThis()

		// Return appropriate string representation based on type
		switch thisValue.Type() {
		case vm.TypeNull:
			return vm.NewString("[object Null]")
		case vm.TypeUndefined:
			return vm.NewString("[object Undefined]")
		case vm.TypeBoolean:
			return vm.NewString("[object Boolean]")
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return vm.NewString("[object Number]")
		case vm.TypeString:
			return vm.NewString("[object String]")
		case vm.TypeArray:
			return vm.NewString("[object Array]")
		case vm.TypeFunction, vm.TypeNativeFunction, vm.TypeClosure:
			return vm.NewString("[object Function]")
		default:
			return vm.NewString("[object Object]")
		}
	}))

	objectProto.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) vm.Value {
		return vmInstance.GetThis() // Return this
	}))

	objectProto.SetOwn("isPrototypeOf", vm.NewNativeFunction(1, false, "isPrototypeOf", func(args []vm.Value) vm.Value {
		if len(args) < 1 {
			return vm.BooleanValue(false)
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
				return vm.BooleanValue(true)
			}

			current = proto
		}

		return vm.BooleanValue(false)
	}))

	// Create Object constructor
	objectCtor := vm.NewNativeFunction(-1, true, "Object", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.NewObject(vm.NewValueFromPlainObject(objectProto))
		}
		arg := args[0]
		if arg.IsObject() {
			return arg
		}
		// TODO: Box primitives properly
		return vm.NewObject(vm.NewValueFromPlainObject(objectProto))
	})

	// Make it a proper constructor with static methods
	if ctorObj := objectCtor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties
		ctorWithProps := vm.NewNativeFunctionWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(objectProto))

		// Add static methods
		ctorPropsObj.Properties.SetOwn("create", vm.NewNativeFunction(1, false, "create", objectCreateImpl))
		ctorPropsObj.Properties.SetOwn("keys", vm.NewNativeFunction(1, false, "keys", objectKeysImpl))
		ctorPropsObj.Properties.SetOwn("getPrototypeOf", vm.NewNativeFunction(1, false, "getPrototypeOf", objectGetPrototypeOfImpl))
		ctorPropsObj.Properties.SetOwn("setPrototypeOf", vm.NewNativeFunction(2, false, "setPrototypeOf", objectSetPrototypeOfImpl))

		objectCtor = ctorWithProps
	}

	// Set constructor property on prototype
	objectProto.SetOwn("constructor", objectCtor)

	// Store in VM
	vmInstance.ObjectPrototype = vm.NewValueFromPlainObject(objectProto)

	// Define globally
	return ctx.DefineGlobal("Object", objectCtor)
}

// Static method implementations

func objectCreateImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	proto := args[0]

	// Check if proto is null or an object
	if proto.Type() != vm.TypeNull && proto.Type() != vm.TypeObject {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	// Create a new object with the specified prototype
	if proto.Type() == vm.TypeNull {
		// For null prototype, create object and set prototype to null
		obj := vm.NewObject(vm.Null)
		if plainObj := obj.AsPlainObject(); plainObj != nil {
			plainObj.SetPrototype(vm.Null)
		}
		return obj
	} else {
		// For object prototype, NewObject handles it correctly
		return vm.NewObject(proto)
	}
}

func objectKeysImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.NewArray()
	}

	obj := args[0]
	if !obj.IsObject() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.NewArray()
	}

	keys := vm.NewArray()
	keysArray := keys.AsArray()

	if plainObj := obj.AsPlainObject(); plainObj != nil {
		for _, key := range plainObj.OwnKeys() {
			keysArray.Append(vm.NewString(key))
		}
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		for _, key := range dictObj.OwnKeys() {
			keysArray.Append(vm.NewString(key))
		}
	}

	return keys
}

func objectGetPrototypeOfImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Undefined
	}

	obj := args[0]

	// For objects with prototypes, return their prototype
	switch obj.Type() {
	case vm.TypeObject:
		// For plain objects, get their actual prototype
		plainObj := obj.AsPlainObject()
		if plainObj != nil {
			return plainObj.GetPrototype()
		}
		return vm.Null
	case vm.TypeArray:
		// For arrays, return Array.prototype if available
		// This will be set up when ArrayInitializer runs
		return vm.Null // TODO: Return proper Array.prototype
	case vm.TypeString:
		// For strings, return String.prototype if available
		return vm.Null // TODO: Return proper String.prototype
	case vm.TypeFunction, vm.TypeClosure:
		// For functions, return Function.prototype if available
		return vm.Null // TODO: Return proper Function.prototype
	default:
		// For primitive values, return null
		return vm.Null
	}
}

func objectSetPrototypeOfImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	obj := args[0]
	proto := args[1]

	// First argument must be an object
	if obj.Type() != vm.TypeObject {
		// TODO: Throw TypeError when error objects are implemented
		return obj // Return the object unchanged as per spec
	}

	// Second argument must be an object or null
	if proto.Type() != vm.TypeNull && proto.Type() != vm.TypeObject {
		// TODO: Throw TypeError when error objects are implemented
		return obj // Return the object unchanged
	}

	// Set the prototype
	if plainObj := obj.AsPlainObject(); plainObj != nil {
		plainObj.SetPrototype(proto)
	} else if dictObj := obj.AsDictObject(); dictObj != nil {
		dictObj.SetPrototype(proto)
	}

	// Return the object
	return obj
}

package builtins

import (
	"fmt"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// registerObjectPrototypeMethods registers Object prototype methods with the type checker
func registerObjectPrototypeMethods() {
	// Register hasOwnProperty method
	RegisterPrototypeMethod("object", "hasOwnProperty",
		types.NewSimpleFunction([]types.Type{types.String}, types.Boolean))

	// Register isPrototypeOf method
	RegisterPrototypeMethod("object", "isPrototypeOf",
		types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean))

	// Register toString method (inherited by all objects)
	RegisterPrototypeMethod("object", "toString",
		types.NewSimpleFunction([]types.Type{}, types.String))

	// Register valueOf method (inherited by all objects)
	RegisterPrototypeMethod("object", "valueOf",
		types.NewSimpleFunction([]types.Type{}, types.Any))
}

// registerObjectConstructor registers the Object constructor and prototype methods
func registerObjectConstructor() {
	// Register Object prototype methods with the type checker
	registerObjectPrototypeMethods()
	// Create Object type with static methods using ObjectType pattern
	objectType := types.NewObjectType().
		WithCallSignature(types.Sig([]types.Type{types.Any}, types.Any).
			WithOptional(true)).
		WithConstructSignature(types.Sig([]types.Type{types.Any}, types.Any).
			WithOptional(true)).
		WithProperty("getPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("create", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("setPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Any))

	// Create the runtime Object function with properties
	objectValue := vm.NewNativeFunctionWithProps(0, true, "Object", objectConstructor)
	objectObj := objectValue.AsNativeFunctionWithProps()

	// Add the getPrototypeOf method to the runtime Object
	objectObj.Properties.SetOwn("getPrototypeOf", vm.NewNativeFunction(1, false, "getPrototypeOf", objectGetPrototypeOf))

	// Add the create method to the runtime Object
	objectObj.Properties.SetOwn("create", vm.NewNativeFunction(1, false, "create", objectCreate))

	// Add the setPrototypeOf method to the runtime Object
	objectObj.Properties.SetOwn("setPrototypeOf", vm.NewNativeFunction(2, false, "setPrototypeOf", objectSetPrototypeOf))

	// Register using registerObject since it's a function with properties
	registerObject("Object", objectValue, objectType)
}

// objectConstructor implements the Object constructor
func objectConstructor(args []vm.Value) vm.Value {
	if len(args) == 0 {
		// new Object() or Object() returns a new empty object
		obj := vm.NewObject(vm.DefaultObjectPrototype)
		return obj
	}

	// Convert the argument to an object
	arg := args[0]
	switch arg.Type() {
	case vm.TypeObject:
		// Already an object, return as-is
		return arg
	case vm.TypeNull, vm.TypeUndefined:
		// null or undefined -> new empty object
		obj := vm.NewObject(vm.DefaultObjectPrototype)
		return obj
	default:
		// Primitive values get boxed (not fully implemented yet)
		// For now, just return a new object
		obj := vm.NewObject(vm.DefaultObjectPrototype)
		return obj
	}
}

// objectGetPrototypeOf implements Object.getPrototypeOf() static method
func objectGetPrototypeOf(args []vm.Value) vm.Value {
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
		if vm.ArrayPrototype != nil {
			return vm.NewValueFromPlainObject(vm.ArrayPrototype)
		}
		return vm.DefaultObjectPrototype
	case vm.TypeString:
		// For strings, return String.prototype if available
		if vm.StringPrototype != nil {
			return vm.NewValueFromPlainObject(vm.StringPrototype)
		}
		return vm.Null
	case vm.TypeFunction, vm.TypeClosure:
		// For functions, return Function.prototype if available
		if vm.FunctionPrototype != nil {
			return vm.NewValueFromPlainObject(vm.FunctionPrototype)
		}
		return vm.Null
	default:
		// For primitive values, return null
		return vm.Null
	}
}

// objectCreate implements Object.create() static method
func objectCreate(args []vm.Value) vm.Value {
	if len(args) == 0 {
		// Object.create() requires at least one argument
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
		// For null prototype, we need to create object and then set prototype to null
		obj := vm.NewObject(vm.DefaultObjectPrototype)
		if plainObj := obj.AsPlainObject(); plainObj != nil {
			plainObj.SetPrototype(vm.Null)
		}
		return obj
	} else {
		// For object prototype, NewObject handles it correctly
		return vm.NewObject(proto)
	}
}

// objectSetPrototypeOf implements Object.setPrototypeOf() static method
func objectSetPrototypeOf(args []vm.Value) vm.Value {
	if len(args) < 2 {
		// Object.setPrototypeOf() requires two arguments
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

// objectHasOwnProperty implements Object.prototype.hasOwnProperty
func objectHasOwnProperty(args []vm.Value) vm.Value {

	fmt.Printf("DEBUG objectHasOwnProperty: args=%v\n", args)
	// 'this' should be the first argument for prototype methods
	if len(args) < 2 {
		return vm.BooleanValue(false)
	}

	thisObj := args[0]
	propName := args[1].ToString()

	fmt.Printf("DEBUG objectHasOwnProperty: this=%v, type=%v, propName=%q\n", thisObj.Inspect(), thisObj.Type(), propName)

	switch thisObj.Type() {
	case vm.TypeObject:
		obj := thisObj.AsPlainObject()
		if obj == nil {
			fmt.Printf("DEBUG objectHasOwnProperty: PlainObject is nil\n")
			return vm.BooleanValue(false)
		}
		result := obj.HasOwn(propName)
		fmt.Printf("DEBUG objectHasOwnProperty: PlainObject.HasOwn(%q) = %v\n", propName, result)
		return vm.BooleanValue(result)
	case vm.TypeDictObject:
		dict := thisObj.AsDictObject()
		if dict == nil {
			fmt.Printf("DEBUG objectHasOwnProperty: DictObject is nil\n")
			return vm.BooleanValue(false)
		}
		result := dict.HasOwn(propName)
		fmt.Printf("DEBUG objectHasOwnProperty: DictObject.HasOwn(%q) = %v\n", propName, result)
		return vm.BooleanValue(result)
	default:
		fmt.Printf("DEBUG objectHasOwnProperty: Unsupported type %v\n", thisObj.Type())
		return vm.BooleanValue(false)
	}
}

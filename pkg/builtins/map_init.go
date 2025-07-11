package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type MapInitializer struct{}

func (m *MapInitializer) Name() string {
	return "Map"
}

func (m *MapInitializer) Priority() int {
	return 400 // After Number (350)
}

func (m *MapInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameters K, V for map methods
	kParam := &types.TypeParameter{Name: "K", Constraint: nil, Index: 0}
	vParam := &types.TypeParameter{Name: "V", Constraint: nil, Index: 1}
	kType := &types.TypeParameterType{Parameter: kParam}
	vType := &types.TypeParameterType{Parameter: vParam}
	
	// Create the generic type first (with placeholder body)
	mapType := &types.GenericType{
		Name:           "Map",
		TypeParameters: []*types.TypeParameter{kParam, vParam},
		Body:           nil, // Will be set below
	}

	// Create Map instance type with methods (using type parameters directly)
	mapInstanceType := types.NewObjectType().
		WithProperty("set", types.NewSimpleFunction([]types.Type{kType, vType}, mapType)).  // Return this for chaining  
		WithProperty("get", types.NewSimpleFunction([]types.Type{kType}, types.NewUnionType(vType, types.Undefined))).
		WithProperty("has", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("size", types.Number)

	// Now set the body of the generic type
	mapType.Body = mapInstanceType

	// Create Map.prototype type for runtime (same structure)
	mapProtoType := types.NewObjectType().
		WithProperty("set", types.NewSimpleFunction([]types.Type{kType, vType}, mapType)).  // Return this for chaining  
		WithProperty("get", types.NewSimpleFunction([]types.Type{kType}, types.NewUnionType(vType, types.Undefined))).
		WithProperty("has", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("size", types.Number)

	// Register map primitive prototype
	ctx.SetPrimitivePrototype("map", mapProtoType)

	// Create Map constructor type - use a generic constructor
	mapCtorType := &types.GenericType{
		Name:           "Map",
		TypeParameters: []*types.TypeParameter{kParam, vParam},
		Body:           types.NewSimpleFunction([]types.Type{}, mapType),
	}

	// Define Map constructor in global environment
	err := ctx.DefineGlobal("Map", mapCtorType)
	if err != nil {
		return err
	}
	
	// Also define the type alias for type annotations like Map<string, number>
	return ctx.DefineTypeAlias("Map", mapType)
}

// createMapMethod creates a generic method with K, V type parameters
func (m *MapInitializer) createMapMethod(name string, kParam, vParam *types.TypeParameter, methodType types.Type) types.Type {
	return &types.GenericType{
		Name:           name,
		TypeParameters: []*types.TypeParameter{kParam, vParam},
		Body:           methodType,
	}
}

func (m *MapInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Map.prototype inheriting from Object.prototype
	mapProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Map prototype methods
	mapProto.SetOwn("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		
		if thisMap.Type() != vm.TypeMap {
			// TODO: Should throw TypeError
			return vm.Undefined, nil
		}
		
		if len(args) < 2 {
			return thisMap, nil // Return the map for chaining
		}
		
		mapObj := thisMap.AsMap()
		mapObj.Set(args[0], args[1])
		return thisMap, nil // Return the map for chaining
	}))

	mapProto.SetOwn("get", vm.NewNativeFunction(1, false, "get", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, nil
		}
		
		if len(args) < 1 {
			return vm.Undefined, nil
		}
		
		mapObj := thisMap.AsMap()
		return mapObj.Get(args[0]), nil
	}))

	mapProto.SetOwn("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		
		if thisMap.Type() != vm.TypeMap {
			return vm.BooleanValue(false), nil
		}
		
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		
		mapObj := thisMap.AsMap()
		return vm.BooleanValue(mapObj.Has(args[0])), nil
	}))

	mapProto.SetOwn("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		
		if thisMap.Type() != vm.TypeMap {
			return vm.BooleanValue(false), nil
		}
		
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		
		mapObj := thisMap.AsMap()
		return vm.BooleanValue(mapObj.Delete(args[0])), nil
	}))

	mapProto.SetOwn("clear", vm.NewNativeFunction(0, false, "clear", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, nil
		}
		
		mapObj := thisMap.AsMap()
		mapObj.Clear()
		return vm.Undefined, nil
	}))

	// Set Map.prototype
	vmInstance.MapPrototype = vm.NewValueFromPlainObject(mapProto)

	// Create Map constructor function
	mapConstructor := vm.NewNativeFunctionWithProps(0, false, "Map", func(args []vm.Value) (vm.Value, error) {
		// Create new Map instance
		return vm.NewMap(), nil
	})

	// Add prototype property
	mapConstructor.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vmInstance.MapPrototype)

	// Define Map constructor in global scope
	return ctx.DefineGlobal("Map", mapConstructor)
}
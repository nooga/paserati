package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type SetInitializer struct{}

func (s *SetInitializer) Name() string {
	return "Set"
}

func (s *SetInitializer) Priority() int {
	return 410 // After Map (400)
}

func (s *SetInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for set methods
	tParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}
	
	// Create the generic type first (with placeholder body)
	setType := &types.GenericType{
		Name:           "Set",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           nil, // Will be set below
	}

	// Create Set instance type with methods (using type parameters directly)
	setInstanceType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, setType)).  // Return this for chaining
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("size", types.Number)

	// Now set the body of the generic type
	setType.Body = setInstanceType

	// Create Set.prototype type for runtime (same structure)
	setProtoType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, setType)).  // Return this for chaining
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("size", types.Number)

	// Register set primitive prototype
	ctx.SetPrimitivePrototype("set", setProtoType)

	// Create Set constructor type - use a generic constructor
	setCtorType := &types.GenericType{
		Name:           "Set",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           types.NewSimpleFunction([]types.Type{}, setType),
	}

	// Define Set constructor in global environment
	err := ctx.DefineGlobal("Set", setCtorType)
	if err != nil {
		return err
	}
	
	// Also define the type alias for type annotations like Set<string>
	return ctx.DefineTypeAlias("Set", setType)
}

// createSetMethod creates a generic method with T type parameter
func (s *SetInitializer) createSetMethod(name string, tParam *types.TypeParameter, methodType types.Type) types.Type {
	return &types.GenericType{
		Name:           name,
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           methodType,
	}
}

func (s *SetInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Set.prototype inheriting from Object.prototype
	setProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Set prototype methods
	setProto.SetOwn("add", vm.NewNativeFunction(1, false, "add", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		
		if thisSet.Type() != vm.TypeSet {
			// TODO: Should throw TypeError
			return vm.Undefined, nil
		}
		
		if len(args) < 1 {
			return thisSet, nil // Return the set for chaining
		}
		
		setObj := thisSet.AsSet()
		setObj.Add(args[0])
		return thisSet, nil // Return the set for chaining
	}))

	setProto.SetOwn("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		
		if thisSet.Type() != vm.TypeSet {
			return vm.BooleanValue(false), nil
		}
		
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		
		setObj := thisSet.AsSet()
		return vm.BooleanValue(setObj.Has(args[0])), nil
	}))

	setProto.SetOwn("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		
		if thisSet.Type() != vm.TypeSet {
			return vm.BooleanValue(false), nil
		}
		
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		
		setObj := thisSet.AsSet()
		return vm.BooleanValue(setObj.Delete(args[0])), nil
	}))

	setProto.SetOwn("clear", vm.NewNativeFunction(0, false, "clear", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, nil
		}
		
		setObj := thisSet.AsSet()
		setObj.Clear()
		return vm.Undefined, nil
	}))

	// Set Set.prototype
	vmInstance.SetPrototype = vm.NewValueFromPlainObject(setProto)

	// Create Set constructor function
	setConstructor := vm.NewNativeFunctionWithProps(0, false, "Set", func(args []vm.Value) (vm.Value, error) {
		// Create new Set instance
		return vm.NewSet(), nil
	})

	// Add prototype property
	setConstructor.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vmInstance.SetPrototype)

	// Define Set constructor in global scope
	return ctx.DefineGlobal("Set", setConstructor)
}
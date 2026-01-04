package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// Priority for Paserati (after standard globals)
const PriorityPaserati = 200

type PaseratiInitializer struct{}

func (p *PaseratiInitializer) Name() string {
	return "Paserati"
}

func (p *PaseratiInitializer) Priority() int {
	return PriorityPaserati
}

func (p *PaseratiInitializer) InitTypes(ctx *TypeContext) error {
	// Create Type interface - represents a runtime type descriptor
	typeInterface := types.NewObjectType().
		WithProperty("kind", types.String).
		WithOptionalProperty("name", types.String).
		WithOptionalProperty("properties", types.Any). // For object types
		WithOptionalProperty("elementType", types.Any). // For array types
		WithOptionalProperty("types", types.Any). // For union types
		WithOptionalProperty("parameters", types.Any). // For function types
		WithOptionalProperty("returnType", types.Any) // For function types

	// Define Type interface for users
	if err := ctx.DefineTypeAlias("Type", typeInterface); err != nil {
		return err
	}

	// Create the Paserati namespace type
	// reflect<T>() is a compile-time intrinsic that returns a Type descriptor
	// We use a marker type that the checker will recognize
	reflectMethodType := types.NewObjectType().WithCallSignature(&types.Signature{
		ParameterTypes: []types.Type{},
		ReturnType:     typeInterface,
	})
	// Mark this as an intrinsic
	reflectMethodType.IsReflectIntrinsic = true

	paseratiType := types.NewObjectType().
		WithProperty("reflect", reflectMethodType)

	// Define Paserati namespace in global environment
	return ctx.DefineGlobal("Paserati", paseratiType)
}

func (p *PaseratiInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Paserati object
	paseratiObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// The reflect method is a placeholder - actual work is done by the compiler
	// If somehow called at runtime, it returns an error
	paseratiObj.SetOwnNonEnumerable("reflect", vm.NewNativeFunction(0, false, "reflect", func(args []vm.Value) (vm.Value, error) {
		// This should never be called - the compiler replaces it
		return vm.Undefined, nil
	}))

	// Register Paserati object as global
	return ctx.DefineGlobal("Paserati", vm.NewValueFromPlainObject(paseratiObj))
}

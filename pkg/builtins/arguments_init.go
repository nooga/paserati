package builtins

import (
	"paserati/pkg/types"
)

type ArgumentsInitializer struct{}

func (a *ArgumentsInitializer) Name() string {
	return "Arguments"
}

func (a *ArgumentsInitializer) Priority() int {
	return PriorityArguments // Will need to define this
}

func (a *ArgumentsInitializer) InitTypes(ctx *TypeContext) error {
	// Create IArguments interface type - array-like object with length and indexed access
	// In TypeScript, the arguments object implements IArguments interface
	iArgumentsType := types.NewObjectType().
		WithProperty("length", types.Number).
		WithProperty("callee", types.NewSimpleFunction([]types.Type{}, types.Any))

	// Add index signature for numeric access: [index: number]: any
	indexSig := &types.IndexSignature{
		KeyType:   types.Number,
		ValueType: types.Any,
	}
	iArgumentsType.IndexSignatures = append(iArgumentsType.IndexSignatures, indexSig)

	// Define the IArguments type globally  
	err := ctx.DefineGlobal("IArguments", iArgumentsType)
	if err != nil {
		return err
	}

	return nil
}

func (a *ArgumentsInitializer) InitRuntime(ctx *RuntimeContext) error {
	// Arguments objects don't have a global constructor like Array
	// They are created on-demand by the OpGetArguments instruction
	// We don't need to set up a global constructor, just the prototype

	// For now, arguments objects will inherit directly from Object.prototype
	// This is sufficient since they get length and indexed access from VM implementation
	// The arguments object is array-like but doesn't inherit from Array.prototype

	return nil
}
package builtins

import (
	"math"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"time"
)

type GlobalsInitializer struct{}

func (g *GlobalsInitializer) Name() string {
	return "Globals"
}

func (g *GlobalsInitializer) Priority() int {
	return 10 // High priority, should be available early
}

func (g *GlobalsInitializer) InitTypes(ctx *TypeContext) error {
	// Define basic type aliases for TypeScript primitive types
	if err := ctx.DefineTypeAlias("number", types.Number); err != nil {
		return err
	}
	if err := ctx.DefineTypeAlias("string", types.String); err != nil {
		return err
	}
	if err := ctx.DefineTypeAlias("boolean", types.Boolean); err != nil {
		return err
	}
	if err := ctx.DefineTypeAlias("null", types.Null); err != nil {
		return err
	}
	if err := ctx.DefineTypeAlias("undefined", types.Undefined); err != nil {
		return err
	}
	if err := ctx.DefineTypeAlias("any", types.Any); err != nil {
		return err
	}
	if err := ctx.DefineTypeAlias("unknown", types.Unknown); err != nil {
		return err
	}
	if err := ctx.DefineTypeAlias("never", types.Never); err != nil {
		return err
	}
	if err := ctx.DefineTypeAlias("void", types.Void); err != nil {
		return err
	}

	// Define global constants
	if err := ctx.DefineGlobal("Infinity", types.Number); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("NaN", types.Number); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("undefined", types.Undefined); err != nil {
		return err
	}

	// Add clock function for backward compatibility
	clockFunctionType := types.NewSimpleFunction([]types.Type{}, types.Number)
	return ctx.DefineGlobal("clock", clockFunctionType)
}

func (g *GlobalsInitializer) InitRuntime(ctx *RuntimeContext) error {
	// Define global constants
	if err := ctx.DefineGlobal("Infinity", vm.NumberValue(math.Inf(1))); err != nil {
		return err
	}

	if err := ctx.DefineGlobal("NaN", vm.NumberValue(math.NaN())); err != nil {
		return err
	}

	if err := ctx.DefineGlobal("undefined", vm.Undefined); err != nil {
		return err
	}

	// Add clock function for backward compatibility (returns current time in milliseconds)
	clockFunc := vm.NewNativeFunction(0, false, "clock", func(args []vm.Value) vm.Value {
		return vm.NumberValue(float64(time.Now().UnixMilli()))
	})

	return ctx.DefineGlobal("clock", clockFunc)
}

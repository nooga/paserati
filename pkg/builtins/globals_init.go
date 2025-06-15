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
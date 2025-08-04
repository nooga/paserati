package builtins

import (
	"math"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strconv"
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
	if err := ctx.DefineGlobal("clock", clockFunctionType); err != nil {
		return err
	}

	// Add parseInt function (second parameter is optional)
	parseIntFunctionType := types.NewSimpleFunction([]types.Type{types.String}, types.Number)
	if err := ctx.DefineGlobal("parseInt", parseIntFunctionType); err != nil {
		return err
	}

	// Add isNaN function
	isNaNFunctionType := types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)
	if err := ctx.DefineGlobal("isNaN", isNaNFunctionType); err != nil {
		return err
	}

	// Add eval function
	evalFunctionType := types.NewSimpleFunction([]types.Type{types.String}, types.Any)
	if err := ctx.DefineGlobal("eval", evalFunctionType); err != nil {
		return err
	}

	// Add globalThis (refers to the global object)
	return ctx.DefineGlobal("globalThis", types.Any)
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
	clockFunc := vm.NewNativeFunction(0, false, "clock", func(args []vm.Value) (vm.Value, error) {
		return vm.NumberValue(float64(time.Now().UnixMilli())), nil
	})

	if err := ctx.DefineGlobal("clock", clockFunc); err != nil {
		return err
	}

	// Add parseInt function implementation
	parseIntFunc := vm.NewNativeFunction(1, false, "parseInt", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}

		str := args[0].ToString()
		var radix int64 = 10

		if len(args) > 1 && args[1].IsNumber() {
			radix = int64(args[1].ToFloat())
			if radix < 2 || radix > 36 {
				return vm.NumberValue(math.NaN()), nil
			}
		}

		// Try to parse the string as an integer with the given radix
		if result, err := strconv.ParseInt(str, int(radix), 64); err == nil {
			return vm.NumberValue(float64(result)), nil
		}

		// If parsing fails, return NaN
		return vm.NumberValue(math.NaN()), nil
	})

	if err := ctx.DefineGlobal("parseInt", parseIntFunc); err != nil {
		return err
	}

	// Add isNaN function implementation
	isNaNFunc := vm.NewNativeFunction(1, false, "isNaN", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.BooleanValue(true), nil // isNaN(undefined) is true
		}

		val := args[0]
		// Convert to number first (like JavaScript does)
		numVal := val.ToFloat()
		return vm.BooleanValue(math.IsNaN(numVal)), nil
	})

	if err := ctx.DefineGlobal("isNaN", isNaNFunc); err != nil {
		return err
	}

	// Add eval function implementation (simplified - just returns undefined for now)
	evalFunc := vm.NewNativeFunction(1, false, "eval", func(args []vm.Value) (vm.Value, error) {
		// TODO: Implement proper eval functionality
		// For now, return undefined to avoid undefined variable errors
		return vm.Undefined, nil
	})

	if err := ctx.DefineGlobal("eval", evalFunc); err != nil {
		return err
	}

	// Add globalThis as a reference to the global object
	// For now, create an empty object - in a real implementation this would be the global scope
	globalThisObj := vm.NewObject(vm.Null)
	return ctx.DefineGlobal("globalThis", vm.NewValueFromPlainObject(globalThisObj.AsPlainObject()))
}

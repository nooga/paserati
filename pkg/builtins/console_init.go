package builtins

import (
	"fmt"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"time"
)

type ConsoleInitializer struct{}

func (c *ConsoleInitializer) Name() string {
	return "console"
}

func (c *ConsoleInitializer) Priority() int {
	return PriorityConsole // 102 - After JSON
}

func (c *ConsoleInitializer) InitTypes(ctx *TypeContext) error {
	// Create console namespace type with all methods
	consoleType := types.NewObjectType().
		WithVariadicProperty("log", []types.Type{}, types.Undefined, &types.ArrayType{ElementType: types.Any}).
		WithVariadicProperty("error", []types.Type{}, types.Undefined, &types.ArrayType{ElementType: types.Any}).
		WithVariadicProperty("warn", []types.Type{}, types.Undefined, &types.ArrayType{ElementType: types.Any}).
		WithVariadicProperty("info", []types.Type{}, types.Undefined, &types.ArrayType{ElementType: types.Any}).
		WithVariadicProperty("debug", []types.Type{}, types.Undefined, &types.ArrayType{ElementType: types.Any}).
		WithVariadicProperty("trace", []types.Type{}, types.Undefined, &types.ArrayType{ElementType: types.Any}).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Undefined)).
		WithProperty("count", types.NewSimpleFunction([]types.Type{types.String}, types.Undefined)).
		WithProperty("countReset", types.NewSimpleFunction([]types.Type{types.String}, types.Undefined)).
		WithProperty("time", types.NewSimpleFunction([]types.Type{types.String}, types.Undefined)).
		WithProperty("timeEnd", types.NewSimpleFunction([]types.Type{types.String}, types.Undefined)).
		WithVariadicProperty("group", []types.Type{}, types.Undefined, &types.ArrayType{ElementType: types.Any}).
		WithVariadicProperty("groupCollapsed", []types.Type{}, types.Undefined, &types.ArrayType{ElementType: types.Any}).
		WithProperty("groupEnd", types.NewSimpleFunction([]types.Type{}, types.Undefined))

	// Define console namespace in global environment
	return ctx.DefineGlobal("console", consoleType)
}

func (c *ConsoleInitializer) InitRuntime(ctx *RuntimeContext) error {
	// Create console object
	consoleObj := vm.NewObject(vm.Null).AsPlainObject()

	// Timer storage for console.time/timeEnd
	timers := make(map[string]time.Time)

	// Helper function to format arguments for console output
	formatArgs := func(args []vm.Value) string {
		if len(args) == 0 {
			return ""
		}

		result := args[0].Inspect()
		for i := 1; i < len(args); i++ {
			result += " " + args[i].Inspect()
		}
		return result
	}

	// Add console methods
	consoleObj.SetOwnNonEnumerable("log", vm.NewNativeFunction(0, true, "log", func(args []vm.Value) (vm.Value, error) {
		fmt.Println(formatArgs(args))
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("error", vm.NewNativeFunction(0, true, "error", func(args []vm.Value) (vm.Value, error) {
		fmt.Printf("ERROR: %s\n", formatArgs(args))
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("warn", vm.NewNativeFunction(0, true, "warn", func(args []vm.Value) (vm.Value, error) {
		fmt.Printf("WARN: %s\n", formatArgs(args))
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("info", vm.NewNativeFunction(0, true, "info", func(args []vm.Value) (vm.Value, error) {
		fmt.Printf("INFO: %s\n", formatArgs(args))
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("debug", vm.NewNativeFunction(0, true, "debug", func(args []vm.Value) (vm.Value, error) {
		fmt.Printf("DEBUG: %s\n", formatArgs(args))
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("trace", vm.NewNativeFunction(0, true, "trace", func(args []vm.Value) (vm.Value, error) {
		fmt.Printf("TRACE: %s\n", formatArgs(args))
		// TODO: Add stack trace
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("clear", vm.NewNativeFunction(0, false, "clear", func(args []vm.Value) (vm.Value, error) {
		// TODO: Implement console clear
		fmt.Print("\033[2J\033[H") // ANSI escape sequence to clear screen
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("count", vm.NewNativeFunction(1, false, "count", func(args []vm.Value) (vm.Value, error) {
		label := "default"
		if len(args) > 0 {
			label = args[0].ToString()
		}
		// TODO: Implement proper counter tracking
		fmt.Printf("%s: 1\n", label)
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("countReset", vm.NewNativeFunction(1, false, "countReset", func(args []vm.Value) (vm.Value, error) {
		label := "default"
		if len(args) > 0 {
			label = args[0].ToString()
		}
		// TODO: Implement proper counter reset
		fmt.Printf("%s: 0\n", label)
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("time", vm.NewNativeFunction(1, false, "time", func(args []vm.Value) (vm.Value, error) {
		label := "default"
		if len(args) > 0 {
			label = args[0].ToString()
		}
		timers[label] = time.Now()
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("timeEnd", vm.NewNativeFunction(1, false, "timeEnd", func(args []vm.Value) (vm.Value, error) {
		label := "default"
		if len(args) > 0 {
			label = args[0].ToString()
		}
		if startTime, exists := timers[label]; exists {
			elapsed := time.Since(startTime)
			fmt.Printf("%s: %.3fms\n", label, float64(elapsed.Nanoseconds())/1000000.0)
			delete(timers, label)
		} else {
			fmt.Printf("Timer '%s' does not exist\n", label)
		}
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("group", vm.NewNativeFunction(0, true, "group", func(args []vm.Value) (vm.Value, error) {
		fmt.Printf("▼ %s\n", formatArgs(args))
		// TODO: Implement proper grouping
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("groupCollapsed", vm.NewNativeFunction(0, true, "groupCollapsed", func(args []vm.Value) (vm.Value, error) {
		fmt.Printf("▶ %s\n", formatArgs(args))
		// TODO: Implement proper grouping
		return vm.Undefined, nil
	}))

	consoleObj.SetOwnNonEnumerable("groupEnd", vm.NewNativeFunction(0, false, "groupEnd", func(args []vm.Value) (vm.Value, error) {
		// TODO: Implement proper group ending
		return vm.Undefined, nil
	}))

	// Register console object as global
	return ctx.DefineGlobal("console", vm.NewValueFromPlainObject(consoleObj))
}

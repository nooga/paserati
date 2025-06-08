package builtins

import (
	"fmt"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"time"
)

// registerConsole creates and registers the console object with its methods
func registerConsole() {
	// Create the console object as a PlainObject
	consoleObj := vm.NewObject(vm.Undefined)
	console := consoleObj.AsPlainObject()

	// Register all console methods
	console.SetOwn("log", vm.NewNativeFunction(-1, true, "log", consoleLogImpl))
	console.SetOwn("error", vm.NewNativeFunction(-1, true, "error", consoleErrorImpl))
	console.SetOwn("warn", vm.NewNativeFunction(-1, true, "warn", consoleWarnImpl))
	console.SetOwn("info", vm.NewNativeFunction(-1, true, "info", consoleInfoImpl))
	console.SetOwn("debug", vm.NewNativeFunction(-1, true, "debug", consoleDebugImpl))
	console.SetOwn("trace", vm.NewNativeFunction(-1, true, "trace", consoleTraceImpl))
	console.SetOwn("clear", vm.NewNativeFunction(0, false, "clear", consoleClearImpl))
	console.SetOwn("count", vm.NewNativeFunction(-1, true, "count", consoleCountImpl))
	console.SetOwn("countReset", vm.NewNativeFunction(-1, true, "countReset", consoleCountResetImpl))
	console.SetOwn("time", vm.NewNativeFunction(-1, true, "time", consoleTimeImpl))
	console.SetOwn("timeEnd", vm.NewNativeFunction(-1, true, "timeEnd", consoleTimeEndImpl))
	console.SetOwn("group", vm.NewNativeFunction(-1, true, "group", consoleGroupImpl))
	console.SetOwn("groupCollapsed", vm.NewNativeFunction(-1, true, "groupCollapsed", consoleGroupCollapsedImpl))
	console.SetOwn("groupEnd", vm.NewNativeFunction(0, false, "groupEnd", consoleGroupEndImpl))

	// Define the type for console object using the smart constructor pattern
	consoleType := types.NewObjectType().
		// Variadic console methods
		WithProperty("log", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("error", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("warn", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("info", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("debug", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("trace", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("count", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("countReset", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("time", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("timeEnd", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("group", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		WithProperty("groupCollapsed", types.NewVariadicFunction([]types.Type{}, types.Void, &types.ArrayType{ElementType: types.Any})).
		
		// Simple console methods
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("groupEnd", types.NewSimpleFunction([]types.Type{}, types.Void))

	// Register the console object
	registerObject("console", consoleObj, consoleType)
}

// --- Console Method Implementations ---

// consoleLogImpl implements console.log(...args)
func consoleLogImpl(args []vm.Value) vm.Value {
	printConsoleMessage(args, "")
	return vm.Undefined
}

// consoleErrorImpl implements console.error(...args)
func consoleErrorImpl(args []vm.Value) vm.Value {
	printConsoleMessage(args, "ERROR: ")
	return vm.Undefined
}

// consoleWarnImpl implements console.warn(...args)
func consoleWarnImpl(args []vm.Value) vm.Value {
	printConsoleMessage(args, "WARN: ")
	return vm.Undefined
}

// consoleInfoImpl implements console.info(...args)
func consoleInfoImpl(args []vm.Value) vm.Value {
	printConsoleMessage(args, "INFO: ")
	return vm.Undefined
}

// consoleDebugImpl implements console.debug(...args)
func consoleDebugImpl(args []vm.Value) vm.Value {
	printConsoleMessage(args, "DEBUG: ")
	return vm.Undefined
}

// consoleTraceImpl implements console.trace(...args)
func consoleTraceImpl(args []vm.Value) vm.Value {
	printConsoleMessage(args, "TRACE: ")
	// TODO: Add stack trace information when available
	return vm.Undefined
}

// consoleClearImpl implements console.clear()
func consoleClearImpl(args []vm.Value) vm.Value {
	// Print ANSI clear screen sequence
	fmt.Print("\033[2J\033[H")
	return vm.Undefined
}

// Simple counter storage for console.count
var counters = make(map[string]int)

// consoleCountImpl implements console.count(label?)
func consoleCountImpl(args []vm.Value) vm.Value {
	label := "default"
	if len(args) > 0 {
		label = args[0].ToString()
	}

	counters[label]++
	fmt.Printf("%s: %d\n", label, counters[label])
	return vm.Undefined
}

// consoleCountResetImpl implements console.countReset(label?)
func consoleCountResetImpl(args []vm.Value) vm.Value {
	label := "default"
	if len(args) > 0 {
		label = args[0].ToString()
	}

	delete(counters, label)
	return vm.Undefined
}

// Simple timer storage for console.time/timeEnd
var timers = make(map[string]int64)

// consoleTimeImpl implements console.time(label?)
func consoleTimeImpl(args []vm.Value) vm.Value {
	label := "default"
	if len(args) > 0 {
		label = args[0].ToString()
	}

	// Store current time in nanoseconds
	timers[label] = getNow()
	return vm.Undefined
}

// consoleTimeEndImpl implements console.timeEnd(label?)
func consoleTimeEndImpl(args []vm.Value) vm.Value {
	label := "default"
	if len(args) > 0 {
		label = args[0].ToString()
	}

	startTime, exists := timers[label]
	if !exists {
		fmt.Printf("Timer '%s' does not exist\n", label)
		return vm.Undefined
	}

	elapsed := float64(getNow()-startTime) / 1e6 // Convert to milliseconds
	fmt.Printf("%s: %.3fms\n", label, elapsed)
	delete(timers, label)
	return vm.Undefined
}

// Simple group indentation level
var groupLevel = 0

// consoleGroupImpl implements console.group(...args)
func consoleGroupImpl(args []vm.Value) vm.Value {
	if len(args) > 0 {
		printConsoleMessage(args, "")
	}
	groupLevel++
	return vm.Undefined
}

// consoleGroupCollapsedImpl implements console.groupCollapsed(...args)
func consoleGroupCollapsedImpl(args []vm.Value) vm.Value {
	// For our simple implementation, treat same as group
	return consoleGroupImpl(args)
}

// consoleGroupEndImpl implements console.groupEnd()
func consoleGroupEndImpl(args []vm.Value) vm.Value {
	if groupLevel > 0 {
		groupLevel--
	}
	return vm.Undefined
}

// --- Helper Functions ---

// printConsoleMessage is a helper function that formats and prints console messages
func printConsoleMessage(args []vm.Value, prefix string) {
	// Add indentation for groups
	indent := ""
	for i := 0; i < groupLevel; i++ {
		indent += "  "
	}

	// Convert all arguments to strings using Inspect() for better formatting
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = arg.Inspect()
	}

	// Print with space separation, followed by newline
	if len(parts) > 0 {
		fmt.Print(indent + prefix)
		for i, part := range parts {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(part)
		}
	} else if prefix != "" {
		fmt.Print(indent + prefix)
	}
	fmt.Println()
}

// getNow returns current time in nanoseconds
func getNow() int64 {
	return time.Now().UnixNano()
}

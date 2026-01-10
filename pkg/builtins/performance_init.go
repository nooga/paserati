package builtins

import (
	"time"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// PerformanceInitializer implements the Performance builtin
type PerformanceInitializer struct{}

func (p *PerformanceInitializer) Name() string {
	return "performance"
}

func (p *PerformanceInitializer) Priority() int {
	return 600 // After core objects
}

func (p *PerformanceInitializer) InitTypes(ctx *TypeContext) error {
	// Create PerformanceEntry interface type
	performanceEntryType := types.NewObjectType().
		WithProperty("name", types.String).
		WithProperty("entryType", types.String).
		WithProperty("startTime", types.Number).
		WithProperty("duration", types.Number)

	// Create Performance object type
	performanceType := types.NewObjectType().
		WithProperty("now", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("mark", types.NewSimpleFunction([]types.Type{types.String}, types.Void)).
		WithProperty("measure", types.NewOptionalFunction([]types.Type{types.String, types.String, types.String}, types.Void, []bool{false, true, true})).
		WithProperty("getEntriesByType", types.NewSimpleFunction([]types.Type{types.String}, &types.ArrayType{ElementType: performanceEntryType})).
		WithProperty("getEntriesByName", types.NewOptionalFunction([]types.Type{types.String, types.String}, &types.ArrayType{ElementType: performanceEntryType}, []bool{false, true})).
		WithProperty("clearMarks", types.NewOptionalFunction([]types.Type{types.String}, types.Void, []bool{true})).
		WithProperty("clearMeasures", types.NewOptionalFunction([]types.Type{types.String}, types.Void, []bool{true}))

	// Define the performance object globally
	return ctx.DefineGlobal("performance", performanceType)
}

// PerformanceEntry represents a performance measurement
type PerformanceEntry struct {
	Name      string
	EntryType string
	StartTime float64
	Duration  float64
}

// Global performance state
var (
	performanceOrigin   = time.Now()
	performanceMarks    = make(map[string]float64)
	performanceMeasures = make(map[string]PerformanceEntry)
)

func (p *PerformanceInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create performance object
	performanceObj := vm.NewObject(objectProto).AsPlainObject()

	// performance.now() - High-resolution timestamp
	performanceObj.SetOwnNonEnumerable("now", vm.NewNativeFunction(0, false, "now", func(args []vm.Value) (vm.Value, error) {
		// Return milliseconds since performance origin with sub-millisecond precision
		elapsed := time.Since(performanceOrigin)
		return vm.NumberValue(float64(elapsed.Nanoseconds()) / 1e6), nil
	}))

	// performance.mark() - Create a named timestamp
	performanceObj.SetOwnNonEnumerable("mark", vm.NewNativeFunction(1, false, "mark", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, nil
		}

		markName := args[0].ToString()
		elapsed := time.Since(performanceOrigin)
		timestamp := float64(elapsed.Nanoseconds()) / 1e6

		performanceMarks[markName] = timestamp
		return vm.Undefined, nil
	}))

	// performance.measure() - Measure between two marks or from start
	performanceObj.SetOwnNonEnumerable("measure", vm.NewNativeFunction(3, false, "measure", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, nil
		}

		measureName := args[0].ToString()
		var startTime, endTime float64

		if len(args) >= 2 && !args[1].IsUndefined() {
			// Start mark specified
			startMarkName := args[1].ToString()
			if start, exists := performanceMarks[startMarkName]; exists {
				startTime = start
			} else {
				// If mark doesn't exist, use 0 (performance origin)
				startTime = 0
			}
		} else {
			// No start mark, measure from origin
			startTime = 0
		}

		if len(args) >= 3 && !args[2].IsUndefined() {
			// End mark specified
			endMarkName := args[2].ToString()
			if end, exists := performanceMarks[endMarkName]; exists {
				endTime = end
			} else {
				// If mark doesn't exist, use current time
				elapsed := time.Since(performanceOrigin)
				endTime = float64(elapsed.Nanoseconds()) / 1e6
			}
		} else {
			// No end mark, use current time
			elapsed := time.Since(performanceOrigin)
			endTime = float64(elapsed.Nanoseconds()) / 1e6
		}

		duration := endTime - startTime
		entry := PerformanceEntry{
			Name:      measureName,
			EntryType: "measure",
			StartTime: startTime,
			Duration:  duration,
		}

		performanceMeasures[measureName] = entry
		return vm.Undefined, nil
	}))

	// performance.getEntriesByType() - Get entries by type
	performanceObj.SetOwnNonEnumerable("getEntriesByType", vm.NewNativeFunction(1, false, "getEntriesByType", func(args []vm.Value) (vm.Value, error) {
		result := vm.NewArray()
		resultArray := result.AsArray()

		if len(args) < 1 {
			return result, nil
		}

		entryType := args[0].ToString()

		if entryType == "mark" {
			for name, startTime := range performanceMarks {
				entry := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				entry.SetOwnNonEnumerable("name", vm.NewString(name))
				entry.SetOwnNonEnumerable("entryType", vm.NewString("mark"))
				entry.SetOwnNonEnumerable("startTime", vm.NumberValue(startTime))
				entry.SetOwnNonEnumerable("duration", vm.NumberValue(0))
				resultArray.Append(vm.NewValueFromPlainObject(entry))
			}
		} else if entryType == "measure" {
			for _, measure := range performanceMeasures {
				entry := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				entry.SetOwnNonEnumerable("name", vm.NewString(measure.Name))
				entry.SetOwnNonEnumerable("entryType", vm.NewString("measure"))
				entry.SetOwnNonEnumerable("startTime", vm.NumberValue(measure.StartTime))
				entry.SetOwnNonEnumerable("duration", vm.NumberValue(measure.Duration))
				resultArray.Append(vm.NewValueFromPlainObject(entry))
			}
		}

		return result, nil
	}))

	// performance.getEntriesByName() - Get entries by name
	performanceObj.SetOwnNonEnumerable("getEntriesByName", vm.NewNativeFunction(2, false, "getEntriesByName", func(args []vm.Value) (vm.Value, error) {
		result := vm.NewArray()
		resultArray := result.AsArray()

		if len(args) < 1 {
			return result, nil
		}

		name := args[0].ToString()
		var entryType string
		if len(args) >= 2 {
			entryType = args[1].ToString()
		}

		// Check marks
		if entryType == "" || entryType == "mark" {
			if startTime, exists := performanceMarks[name]; exists {
				entry := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				entry.SetOwnNonEnumerable("name", vm.NewString(name))
				entry.SetOwnNonEnumerable("entryType", vm.NewString("mark"))
				entry.SetOwnNonEnumerable("startTime", vm.NumberValue(startTime))
				entry.SetOwnNonEnumerable("duration", vm.NumberValue(0))
				resultArray.Append(vm.NewValueFromPlainObject(entry))
			}
		}

		// Check measures
		if entryType == "" || entryType == "measure" {
			if measure, exists := performanceMeasures[name]; exists {
				entry := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				entry.SetOwnNonEnumerable("name", vm.NewString(measure.Name))
				entry.SetOwnNonEnumerable("entryType", vm.NewString("measure"))
				entry.SetOwnNonEnumerable("startTime", vm.NumberValue(measure.StartTime))
				entry.SetOwnNonEnumerable("duration", vm.NumberValue(measure.Duration))
				resultArray.Append(vm.NewValueFromPlainObject(entry))
			}
		}

		return result, nil
	}))

	// performance.clearMarks() - Clear marks
	performanceObj.SetOwnNonEnumerable("clearMarks", vm.NewNativeFunction(1, false, "clearMarks", func(args []vm.Value) (vm.Value, error) {
		if len(args) >= 1 && !args[0].IsUndefined() {
			// Clear specific mark
			markName := args[0].ToString()
			delete(performanceMarks, markName)
		} else {
			// Clear all marks
			performanceMarks = make(map[string]float64)
		}
		return vm.Undefined, nil
	}))

	// performance.clearMeasures() - Clear measures
	performanceObj.SetOwnNonEnumerable("clearMeasures", vm.NewNativeFunction(1, false, "clearMeasures", func(args []vm.Value) (vm.Value, error) {
		if len(args) >= 1 && !args[0].IsUndefined() {
			// Clear specific measure
			measureName := args[0].ToString()
			delete(performanceMeasures, measureName)
		} else {
			// Clear all measures
			performanceMeasures = make(map[string]PerformanceEntry)
		}
		return vm.Undefined, nil
	}))

	// Define globally
	return ctx.DefineGlobal("performance", vm.NewValueFromPlainObject(performanceObj))
}

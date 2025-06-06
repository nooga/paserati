package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"time"
)

// registerDate registers the Date constructor and prototype methods
func registerDate() {
	// Register Date constructor
	dateCallableType := &types.CallableType{
		CallSignature: &types.FunctionType{
			ParameterTypes:    []types.Type{},                                         // Can accept various arguments
			ReturnType:        &types.ObjectType{Properties: map[string]types.Type{}}, // Returns Date object
			IsVariadic:        true,
			RestParameterType: types.Any,
		},
		Properties: map[string]types.Type{
			"now": &types.FunctionType{
				ParameterTypes: []types.Type{},
				ReturnType:     types.Number,
				IsVariadic:     false,
			},
		},
	}

	dateConstructorValue := vm.NewNativeFunctionWithProps(-1, true, "Date", dateConstructor)
	// Add static methods to the constructor
	dateProps := dateConstructorValue.AsNativeFunctionWithProps().Properties
	dateProps.SetOwn("now", vm.NewNativeFunction(0, false, "now", dateNowImpl))

	registerValue("Date", dateConstructorValue, dateCallableType)

	// Register Date prototype methods (both runtime and types)
	registerDatePrototypeMethods()
}

// registerDatePrototypeMethods registers Date prototype methods with both implementations and types
func registerDatePrototypeMethods() {
	// For now, Date prototype methods will be hardcoded in the type checker
	// This is a temporary approach until we have a proper object prototype system
}

// dateConstructor implements the Date() constructor
func dateConstructor(args []vm.Value) vm.Value {
	var t time.Time

	if len(args) == 0 {
		// No arguments: current time
		t = time.Now()
	} else if len(args) == 1 {
		// Single argument: timestamp or date string
		arg := args[0]
		if arg.IsNumber() {
			// Timestamp in milliseconds
			timestamp := int64(arg.ToFloat())
			t = time.Unix(timestamp/1000, (timestamp%1000)*1000000)
		} else {
			// Date string - for now, just use current time
			t = time.Now()
		}
	} else {
		// Multiple arguments: year, month, day, etc.
		year := int(args[0].ToFloat())
		month := int(args[1].ToFloat()) + 1 // JS months are 0-based
		day := 1
		if len(args) > 2 {
			day = int(args[2].ToFloat())
		}
		hour, minute, second := 0, 0, 0
		if len(args) > 3 {
			hour = int(args[3].ToFloat())
		}
		if len(args) > 4 {
			minute = int(args[4].ToFloat())
		}
		if len(args) > 5 {
			second = int(args[5].ToFloat())
		}
		t = time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	}

	// Create a Date object (we'll use a plain object with internal timestamp)
	dateObj := vm.NewObject(vm.Undefined).AsPlainObject()
	dateObj.SetOwn("__timestamp", vm.Number(float64(t.Unix()*1000+int64(t.Nanosecond()/1000000))))
	dateObj.SetOwn("__prototype", vm.NewString("date"))

	// For now, let's just return the object as-is since direct Value construction is complex
	return vm.NewObject(vm.Undefined)
}

// dateNowImpl implements Date.now()
func dateNowImpl(args []vm.Value) vm.Value {
	return vm.Number(float64(time.Now().Unix() * 1000))
}

// dateGetTimeImpl implements Date.prototype.getTime()
func dateGetTimeImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(0)
	}

	thisValue := args[0]
	if !thisValue.IsObject() {
		return vm.Undefined
	}

	dateObj := thisValue.AsPlainObject()
	if timestamp, exists := dateObj.GetOwn("__timestamp"); exists {
		return timestamp
	}

	return vm.Number(0)
}

// dateGetFullYearImpl implements Date.prototype.getFullYear()
func dateGetFullYearImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(0)
	}

	thisValue := args[0]
	if !thisValue.IsObject() {
		return vm.Undefined
	}

	dateObj := thisValue.AsPlainObject()
	if timestamp, exists := dateObj.GetOwn("__timestamp"); exists {
		t := time.Unix(int64(timestamp.ToFloat()/1000), 0)
		return vm.Number(float64(t.Year()))
	}

	return vm.Number(0)
}

// dateGetMonthImpl implements Date.prototype.getMonth()
func dateGetMonthImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(0)
	}

	thisValue := args[0]
	if !thisValue.IsObject() {
		return vm.Undefined
	}

	dateObj := thisValue.AsPlainObject()
	if timestamp, exists := dateObj.GetOwn("__timestamp"); exists {
		t := time.Unix(int64(timestamp.ToFloat()/1000), 0)
		return vm.Number(float64(t.Month() - 1)) // JS months are 0-based
	}

	return vm.Number(0)
}

// dateGetDateImpl implements Date.prototype.getDate()
func dateGetDateImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(0)
	}

	thisValue := args[0]
	if !thisValue.IsObject() {
		return vm.Undefined
	}

	dateObj := thisValue.AsPlainObject()
	if timestamp, exists := dateObj.GetOwn("__timestamp"); exists {
		t := time.Unix(int64(timestamp.ToFloat()/1000), 0)
		return vm.Number(float64(t.Day()))
	}

	return vm.Number(0)
}

// dateToStringImpl implements Date.prototype.toString()
func dateToStringImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.NewString("")
	}

	thisValue := args[0]
	if !thisValue.IsObject() {
		return vm.Undefined
	}

	dateObj := thisValue.AsPlainObject()
	if timestamp, exists := dateObj.GetOwn("__timestamp"); exists {
		t := time.Unix(int64(timestamp.ToFloat()/1000), 0)
		return vm.NewString(t.Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"))
	}

	return vm.NewString("")
}

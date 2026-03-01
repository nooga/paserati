package builtins

import (
	"errors"
	"math"
	"time"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// errDateUnwinding is a sentinel error for ToPrimitive valueOf/toString exceptions in Date setters
var errDateUnwinding = errors.New("VM unwinding")

// dateToNumber converts a value to a number using ToPrimitive (calls valueOf/toString on objects).
// Returns (float64, error) where error is set if valueOf/toString threw an exception.
func dateToNumber(vmInstance *vm.VM, val vm.Value) (float64, error) {
	if val.IsObject() || val.IsCallable() {
		vmInstance.EnterHelperCall()
		prim := vmInstance.ToPrimitive(val, "number")
		vmInstance.ExitHelperCall()
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return math.NaN(), errDateUnwinding
		}
		return prim.ToFloat(), nil
	}
	return val.ToFloat(), nil
}

type DateInitializer struct{}

func (d *DateInitializer) Name() string {
	return "Date"
}

func (d *DateInitializer) Priority() int {
	return 400 // After Object (100), Function (200), String (300), Array (2)
}

func (d *DateInitializer) InitTypes(ctx *TypeContext) error {
	// Create Date.prototype type with all methods
	dateProtoType := types.NewObjectType().
		WithProperty("getTime", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getFullYear", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getMonth", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getDate", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getDay", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getHours", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getMinutes", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getSeconds", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getMilliseconds", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getUTCFullYear", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getUTCMonth", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getUTCDate", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getUTCDay", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getUTCHours", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getUTCMinutes", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getUTCSeconds", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getUTCMilliseconds", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("getTimezoneOffset", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("setTime", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("setFullYear", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number}, types.Number, []bool{false, true, true})).
		WithProperty("setMonth", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Number, []bool{false, true})).
		WithProperty("setDate", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("setHours", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number, types.Number}, types.Number, []bool{false, true, true, true})).
		WithProperty("setMinutes", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number}, types.Number, []bool{false, true, true})).
		WithProperty("setSeconds", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Number, []bool{false, true})).
		WithProperty("setMilliseconds", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("setUTCFullYear", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number}, types.Number, []bool{false, true, true})).
		WithProperty("setUTCMonth", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Number, []bool{false, true})).
		WithProperty("setUTCDate", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("setUTCHours", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number, types.Number}, types.Number, []bool{false, true, true, true})).
		WithProperty("setUTCMinutes", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number}, types.Number, []bool{false, true, true})).
		WithProperty("setUTCSeconds", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Number, []bool{false, true})).
		WithProperty("setUTCMilliseconds", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toISOString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toDateString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toTimeString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toLocaleString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toLocaleDateString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toLocaleTimeString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toUTCString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toJSON", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("constructor", types.Any). // Avoid circular reference, use Any for constructor property
		// Annex B methods
		WithProperty("getYear", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("setYear", types.NewSimpleFunction([]types.Type{types.Number}, types.Number))

	// Create Date constructor type
	dateCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, dateProtoType).                                                                          // Date() -> Date
		WithSimpleCallSignature([]types.Type{types.Number}, dateProtoType).                                                              // Date(timestamp) -> Date
		WithSimpleCallSignature([]types.Type{types.String}, dateProtoType).                                                              // Date(dateString) -> Date
		WithVariadicCallSignature([]types.Type{types.Number, types.Number}, dateProtoType, &types.ArrayType{ElementType: types.Number}). // Date(year, month, ...) -> Date
		WithProperty("now", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("parse", types.NewSimpleFunction([]types.Type{types.String}, types.Number)).
		WithVariadicProperty("UTC", []types.Type{types.Number, types.Number}, types.Number, &types.ArrayType{ElementType: types.Number}).
		WithProperty("prototype", dateProtoType)

	return ctx.DefineGlobal("Date", dateCtorType)
}

func (d *DateInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Date.prototype inheriting from Object.prototype
	dateProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Date prototype methods
	dateProto.SetOwnNonEnumerable("getTime", vm.NewNativeFunction(0, false, "getTime", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(timestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("getFullYear", vm.NewNativeFunction(0, false, "getFullYear", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NumberValue(float64(t.Year())), nil
	}))

	dateProto.SetOwnNonEnumerable("getMonth", vm.NewNativeFunction(0, false, "getMonth", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NumberValue(float64(t.Month() - 1)), nil // JavaScript months are 0-based
	}))

	dateProto.SetOwnNonEnumerable("getDate", vm.NewNativeFunction(0, false, "getDate", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NumberValue(float64(t.Day())), nil
	}))

	dateProto.SetOwnNonEnumerable("getDay", vm.NewNativeFunction(0, false, "getDay", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NumberValue(float64(t.Weekday())), nil // Sunday = 0 in JavaScript
	}))

	dateProto.SetOwnNonEnumerable("getHours", vm.NewNativeFunction(0, false, "getHours", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NumberValue(float64(t.Hour())), nil
	}))

	dateProto.SetOwnNonEnumerable("getMinutes", vm.NewNativeFunction(0, false, "getMinutes", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NumberValue(float64(t.Minute())), nil
	}))

	dateProto.SetOwnNonEnumerable("getSeconds", vm.NewNativeFunction(0, false, "getSeconds", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NumberValue(float64(t.Second())), nil
	}))

	dateProto.SetOwnNonEnumerable("getMilliseconds", vm.NewNativeFunction(0, false, "getMilliseconds", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NumberValue(float64(t.Nanosecond() / 1000000)), nil
	}))

	// UTC getter methods
	dateProto.SetOwnNonEnumerable("getUTCFullYear", vm.NewNativeFunction(0, false, "getUTCFullYear", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		return vm.NumberValue(float64(t.Year())), nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCMonth", vm.NewNativeFunction(0, false, "getUTCMonth", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		return vm.NumberValue(float64(t.Month() - 1)), nil // JavaScript months are 0-based
	}))

	dateProto.SetOwnNonEnumerable("getUTCDate", vm.NewNativeFunction(0, false, "getUTCDate", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		return vm.NumberValue(float64(t.Day())), nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCDay", vm.NewNativeFunction(0, false, "getUTCDay", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		return vm.NumberValue(float64(t.Weekday())), nil // Sunday = 0 in JavaScript
	}))

	dateProto.SetOwnNonEnumerable("getUTCHours", vm.NewNativeFunction(0, false, "getUTCHours", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		return vm.NumberValue(float64(t.Hour())), nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCMinutes", vm.NewNativeFunction(0, false, "getUTCMinutes", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		return vm.NumberValue(float64(t.Minute())), nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCSeconds", vm.NewNativeFunction(0, false, "getUTCSeconds", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		return vm.NumberValue(float64(t.Second())), nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCMilliseconds", vm.NewNativeFunction(0, false, "getUTCMilliseconds", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		return vm.NumberValue(float64(t.Nanosecond() / 1000000)), nil
	}))

	dateProto.SetOwnNonEnumerable("getTimezoneOffset", vm.NewNativeFunction(0, false, "getTimezoneOffset", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		_, offset := t.Zone()
		return vm.NumberValue(float64(-offset / 60)), nil // JavaScript returns minutes, negative for east of UTC
	}))

	// setTime method
	dateProto.SetOwnNonEnumerable("setTime", vm.NewNativeFunction(1, false, "setTime", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		// Step 1: Check this is a Date object
		_, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		// Step 2: Convert argument to number
		var newTimestamp float64
		if len(args) < 1 {
			newTimestamp = math.NaN()
		} else {
			newTimestamp = args[0].ToFloat()
		}
		// Step 3: Set the timestamp
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setDate", vm.NewNativeFunction(1, false, "setDate", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		// Step 1: Get dateObject.[[DateValue]]
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		// Step 2: ToNumber(date) — must call BEFORE checking if timestamp is NaN
		var day float64
		if len(args) < 1 {
			day = math.NaN()
		} else {
			day, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Step 3: If t is NaN, return NaN
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		if math.IsNaN(day) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		newTime := time.Date(t.Year(), t.Month(), int(day), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setMonth", vm.NewNativeFunction(2, false, "setMonth", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		// Step 1: Get dateObject.[[DateValue]]
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		// Step 2: ToNumber all arguments BEFORE checking NaN
		var monthNum float64
		if len(args) < 1 {
			monthNum = math.NaN()
		} else {
			monthNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var dayNum float64
		hasDayArg := len(args) >= 2
		if hasDayArg {
			dayNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Step 3: If t is NaN, return NaN
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(monthNum) || (hasDayArg && math.IsNaN(dayNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		month := time.Month(int(monthNum) + 1)
		day := t.Day()
		if hasDayArg {
			day = int(dayNum)
		}
		newTime := time.Date(t.Year(), month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setFullYear", vm.NewNativeFunction(3, false, "setFullYear", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		// Step 1: Get dateObject.[[DateValue]]
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		// Step 2: ToNumber all arguments BEFORE using timestamp
		var yearNum float64
		if len(args) < 1 {
			yearNum = math.NaN()
		} else {
			yearNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var monthNum float64
		hasMonthArg := len(args) >= 2
		if hasMonthArg {
			monthNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var dayNum float64
		hasDayArg := len(args) >= 3
		if hasDayArg {
			dayNum, err = dateToNumber(vmInstance, args[2])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Step 3: If t is NaN, treat as +0
		var t time.Time
		if math.IsNaN(timestamp) {
			t = time.UnixMilli(0).UTC()
		} else {
			t = time.UnixMilli(int64(timestamp))
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(yearNum) || (hasMonthArg && math.IsNaN(monthNum)) || (hasDayArg && math.IsNaN(dayNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		year := int(yearNum)
		month := t.Month()
		if hasMonthArg {
			month = time.Month(int(monthNum) + 1)
		}
		day := t.Day()
		if hasDayArg {
			day = int(dayNum)
		}
		newTime := time.Date(year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setHours", vm.NewNativeFunction(4, false, "setHours", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		// Step 1: Get dateObject.[[DateValue]]
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		// Step 2: ToNumber all arguments BEFORE checking NaN
		var hourNum float64
		if len(args) < 1 {
			hourNum = math.NaN()
		} else {
			hourNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var minuteNum float64
		hasMinArg := len(args) >= 2
		if hasMinArg {
			minuteNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var secondNum float64
		hasSecArg := len(args) >= 3
		if hasSecArg {
			secondNum, err = dateToNumber(vmInstance, args[2])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var msNum float64
		hasMsArg := len(args) >= 4
		if hasMsArg {
			msNum, err = dateToNumber(vmInstance, args[3])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Step 3: If t is NaN, return NaN
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(hourNum) || (hasMinArg && math.IsNaN(minuteNum)) || (hasSecArg && math.IsNaN(secondNum)) || (hasMsArg && math.IsNaN(msNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		hour := int(hourNum)
		minute := t.Minute()
		if hasMinArg {
			minute = int(minuteNum)
		}
		second := t.Second()
		if hasSecArg {
			second = int(secondNum)
		}
		nanosecond := t.Nanosecond()
		if hasMsArg {
			nanosecond = int(msNum) * 1000000
		}
		newTime := time.Date(t.Year(), t.Month(), t.Day(), hour, minute, second, nanosecond, t.Location())
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setMinutes", vm.NewNativeFunction(3, false, "setMinutes", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		// Step 1: Get dateObject.[[DateValue]]
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		// Step 2: ToNumber all arguments BEFORE checking NaN
		var minuteNum float64
		if len(args) < 1 {
			minuteNum = math.NaN()
		} else {
			minuteNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var secondNum float64
		hasSecArg := len(args) >= 2
		if hasSecArg {
			secondNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var msNum float64
		hasMsArg := len(args) >= 3
		if hasMsArg {
			msNum, err = dateToNumber(vmInstance, args[2])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Step 3: If t is NaN, return NaN
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(minuteNum) || (hasSecArg && math.IsNaN(secondNum)) || (hasMsArg && math.IsNaN(msNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		minute := int(minuteNum)
		second := t.Second()
		if hasSecArg {
			second = int(secondNum)
		}
		nanosecond := t.Nanosecond()
		if hasMsArg {
			nanosecond = int(msNum) * 1000000
		}
		newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, second, nanosecond, t.Location())
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setSeconds", vm.NewNativeFunction(2, false, "setSeconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		// Step 1: Get dateObject.[[DateValue]]
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		// Step 2: ToNumber all arguments BEFORE checking NaN
		var secondNum float64
		if len(args) < 1 {
			secondNum = math.NaN()
		} else {
			secondNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var msNum float64
		hasMsArg := len(args) >= 2
		if hasMsArg {
			msNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Step 3: If t is NaN, return NaN
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(secondNum) || (hasMsArg && math.IsNaN(msNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		second := int(secondNum)
		nanosecond := t.Nanosecond()
		if hasMsArg {
			nanosecond = int(msNum) * 1000000
		}
		newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), second, nanosecond, t.Location())
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setMilliseconds", vm.NewNativeFunction(1, false, "setMilliseconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		// Step 1: Get dateObject.[[DateValue]]
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		// Step 2: ToNumber(ms) BEFORE checking NaN
		var msNum float64
		if len(args) < 1 {
			msNum = math.NaN()
		} else {
			msNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Step 3: If t is NaN, return NaN
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		if math.IsNaN(msNum) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		millisecond := int(msNum) * 1000000
		newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), millisecond, t.Location())
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NewString("Invalid Date"), nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NewString(t.Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")), nil
	}))

	dateProto.SetOwnNonEnumerable("toISOString", vm.NewNativeFunction(0, false, "toISOString", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.Undefined, vmInstance.NewRangeError("Invalid time value")
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		return vm.NewString(t.Format("2006-01-02T15:04:05.000Z")), nil
	}))

	dateProto.SetOwnNonEnumerable("toDateString", vm.NewNativeFunction(0, false, "toDateString", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NewString("Invalid Date"), nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NewString(t.Format("Mon Jan 02 2006")), nil
	}))

	dateProto.SetOwnNonEnumerable("toTimeString", vm.NewNativeFunction(0, false, "toTimeString", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NewString("Invalid Date"), nil
		}
		t := time.UnixMilli(int64(timestamp))
		return vm.NewString(t.Format("15:04:05 GMT-0700 (MST)")), nil
	}))

	dateProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(timestamp), nil
	}))

	// Locale methods
	dateProto.SetOwnNonEnumerable("toLocaleString", vm.NewNativeFunction(0, false, "toLocaleString", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NewString("Invalid Date"), nil
		}
		t := time.UnixMilli(int64(timestamp))
		// Simple locale format - could be enhanced with actual locale support
		return vm.NewString(t.Format("1/2/2006, 3:04:05 PM")), nil
	}))

	dateProto.SetOwnNonEnumerable("toLocaleDateString", vm.NewNativeFunction(0, false, "toLocaleDateString", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NewString("Invalid Date"), nil
		}
		t := time.UnixMilli(int64(timestamp))
		// Simple locale format - could be enhanced with actual locale support
		return vm.NewString(t.Format("1/2/2006")), nil
	}))

	dateProto.SetOwnNonEnumerable("toLocaleTimeString", vm.NewNativeFunction(0, false, "toLocaleTimeString", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NewString("Invalid Date"), nil
		}
		t := time.UnixMilli(int64(timestamp))
		// Simple locale format - could be enhanced with actual locale support
		return vm.NewString(t.Format("3:04:05 PM")), nil
	}))

	// toUTCString - returns date in UTC timezone as RFC 7231 format
	dateProto.SetOwnNonEnumerable("toUTCString", vm.NewNativeFunction(0, false, "toUTCString", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NewString("Invalid Date"), nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		// Format: "Sun, 19 Jan 2025 10:30:00 GMT" (RFC 7231)
		return vm.NewString(t.Format("Mon, 02 Jan 2006 15:04:05 GMT")), nil
	}))

	// toJSON method - Per spec, works on any object with toISOString
	dateProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// Step 1: Let O be ? ToObject(this value)
		objVal, err := vmInstance.ToObject(thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// Step 2: ToPrimitive with hint Number
		tv := vmInstance.ToPrimitive(objVal, "number")
		// Step 3: If tv is Number and not finite, return null
		if tv.IsNumber() {
			numVal := tv.ToFloat()
			if math.IsNaN(numVal) || math.IsInf(numVal, 0) {
				return vm.Null, nil
			}
		}
		// Step 4: Call toISOString method on this value (O, not the original this)
		toISOString, err := vmInstance.GetProperty(objVal, "toISOString")
		if err != nil {
			return vm.Undefined, err
		}
		if !toISOString.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("toISOString is not a function")
		}
		return vmInstance.Call(toISOString, objVal, nil)
	}))

	// Annex B methods
	dateProto.SetOwnNonEnumerable("getYear", vm.NewNativeFunction(0, false, "getYear", func(args []vm.Value) (vm.Value, error) {
		timestamp, err := thisTimeValue(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp))
		// getYear returns year - 1900
		return vm.NumberValue(float64(t.Year() - 1900)), nil
	}))

	dateProto.SetOwnNonEnumerable("setYear", vm.NewNativeFunction(1, false, "setYear", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		dateObj := thisDate.AsPlainObject()
		if dateObj == nil {
			return vm.Undefined, vmInstance.NewTypeError("setYear called on non-Date object")
		}

		// Check if it's a Date object by verifying it has __timestamp__ property
		if _, exists := dateObj.GetOwn("__timestamp__"); !exists {
			return vm.Undefined, vmInstance.NewTypeError("setYear called on non-Date object")
		}

		if len(args) == 0 {
			// Set to NaN if no arguments
			dateObj.SetOwnNonEnumerable("__timestamp__", vm.NaN)
			return vm.NaN, nil
		}

		yearArg := args[0].ToFloat()
		if math.IsNaN(yearArg) {
			dateObj.SetOwnNonEnumerable("__timestamp__", vm.NaN)
			return vm.NaN, nil
		}

		// Get current timestamp
		currentTimestamp, _ := getDateTimestamp(thisDate)
		if math.IsNaN(currentTimestamp) {
			// If date is invalid, initialize to January 1, 1900
			t := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)
			currentTimestamp = float64(t.UnixMilli())
		}

		t := time.UnixMilli(int64(currentTimestamp))

		// Year interpretation: 0-99 means 1900-1999
		year := int(yearArg)
		if year >= 0 && year <= 99 {
			year += 1900
		}

		// Create new time with updated year
		newTime := time.Date(year, t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		newTimestamp := float64(newTime.UnixMilli())

		// Update the date object
		dateObj.SetOwnNonEnumerable("__timestamp__", vm.NumberValue(newTimestamp))
		return vm.NumberValue(newTimestamp), nil
	}))

	// UTC setter methods
	dateProto.SetOwnNonEnumerable("setUTCFullYear", vm.NewNativeFunction(3, false, "setUTCFullYear", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		// ToNumber all arguments first
		var yearNum float64
		if len(args) < 1 {
			yearNum = math.NaN()
		} else {
			yearNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var monthNum float64
		hasMonthArg := len(args) >= 2
		if hasMonthArg {
			monthNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var dayNum float64
		hasDayArg := len(args) >= 3
		if hasDayArg {
			dayNum, err = dateToNumber(vmInstance, args[2])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Per spec: if t is NaN, set t to +0
		var t time.Time
		if math.IsNaN(timestamp) {
			t = time.UnixMilli(0).UTC()
		} else {
			t = time.UnixMilli(int64(timestamp)).UTC()
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(yearNum) || (hasMonthArg && math.IsNaN(monthNum)) || (hasDayArg && math.IsNaN(dayNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		year := int(yearNum)
		month := t.Month()
		if hasMonthArg {
			month = time.Month(int(monthNum) + 1)
		}
		day := t.Day()
		if hasDayArg {
			day = int(dayNum)
		}
		newTime := time.Date(year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCMonth", vm.NewNativeFunction(2, false, "setUTCMonth", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		var monthNum float64
		if len(args) < 1 {
			monthNum = math.NaN()
		} else {
			monthNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var dayNum float64
		hasDayArg := len(args) >= 2
		if hasDayArg {
			dayNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(monthNum) || (hasDayArg && math.IsNaN(dayNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		month := time.Month(int(monthNum) + 1)
		day := t.Day()
		if hasDayArg {
			day = int(dayNum)
		}
		newTime := time.Date(t.Year(), month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCDate", vm.NewNativeFunction(1, false, "setUTCDate", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		var dayNum float64
		if len(args) < 1 {
			dayNum = math.NaN()
		} else {
			dayNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		if math.IsNaN(dayNum) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		newTime := time.Date(t.Year(), t.Month(), int(dayNum), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCHours", vm.NewNativeFunction(4, false, "setUTCHours", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		var hourNum float64
		if len(args) < 1 {
			hourNum = math.NaN()
		} else {
			hourNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var minuteNum float64
		hasMinArg := len(args) >= 2
		if hasMinArg {
			minuteNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var secondNum float64
		hasSecArg := len(args) >= 3
		if hasSecArg {
			secondNum, err = dateToNumber(vmInstance, args[2])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var msNum float64
		hasMsArg := len(args) >= 4
		if hasMsArg {
			msNum, err = dateToNumber(vmInstance, args[3])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(hourNum) || (hasMinArg && math.IsNaN(minuteNum)) || (hasSecArg && math.IsNaN(secondNum)) || (hasMsArg && math.IsNaN(msNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		hour := int(hourNum)
		minute := t.Minute()
		if hasMinArg {
			minute = int(minuteNum)
		}
		second := t.Second()
		if hasSecArg {
			second = int(secondNum)
		}
		nanosecond := t.Nanosecond()
		if hasMsArg {
			nanosecond = int(msNum) * 1000000
		}
		newTime := time.Date(t.Year(), t.Month(), t.Day(), hour, minute, second, nanosecond, time.UTC)
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCMinutes", vm.NewNativeFunction(3, false, "setUTCMinutes", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		var minuteNum float64
		if len(args) < 1 {
			minuteNum = math.NaN()
		} else {
			minuteNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var secondNum float64
		hasSecArg := len(args) >= 2
		if hasSecArg {
			secondNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var msNum float64
		hasMsArg := len(args) >= 3
		if hasMsArg {
			msNum, err = dateToNumber(vmInstance, args[2])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(minuteNum) || (hasSecArg && math.IsNaN(secondNum)) || (hasMsArg && math.IsNaN(msNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		minute := int(minuteNum)
		second := t.Second()
		if hasSecArg {
			second = int(secondNum)
		}
		nanosecond := t.Nanosecond()
		if hasMsArg {
			nanosecond = int(msNum) * 1000000
		}
		newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, second, nanosecond, time.UTC)
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCSeconds", vm.NewNativeFunction(2, false, "setUTCSeconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		var secondNum float64
		if len(args) < 1 {
			secondNum = math.NaN()
		} else {
			secondNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		var msNum float64
		hasMsArg := len(args) >= 2
		if hasMsArg {
			msNum, err = dateToNumber(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		// If any numeric argument is NaN, result is NaN
		if math.IsNaN(secondNum) || (hasMsArg && math.IsNaN(msNum)) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		second := int(secondNum)
		nanosecond := t.Nanosecond()
		if hasMsArg {
			nanosecond = int(msNum) * 1000000
		}
		newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), second, nanosecond, time.UTC)
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCMilliseconds", vm.NewNativeFunction(1, false, "setUTCMilliseconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		timestamp, err := thisTimeValue(vmInstance, thisDate)
		if err != nil {
			return vm.Undefined, err
		}
		var msNum float64
		if len(args) < 1 {
			msNum = math.NaN()
		} else {
			msNum, err = dateToNumber(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if math.IsNaN(timestamp) {
			return vm.NaN, nil
		}
		if math.IsNaN(msNum) {
			setDateTimestamp(thisDate, math.NaN())
			return vm.NaN, nil
		}
		t := time.UnixMilli(int64(timestamp)).UTC()
		nanosecond := int(msNum) * 1000000
		newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), nanosecond, time.UTC)
		newTimestamp := float64(newTime.UnixMilli())
		newTimestamp = setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	// Create Date constructor
	ctorWithProps := vm.NewConstructorWithProps(-1, true, "Date", func(args []vm.Value) (vm.Value, error) {
		// When called as a function (not constructor), return current date/time string
		if !vmInstance.IsConstructorCall() {
			t := time.Now()
			return vm.NewString(t.Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")), nil
		}

		// Per ECMAScript 21.4.2.1 step 4:
		// Let O be ? OrdinaryCreateFromConstructor(NewTarget, "%Date.prototype%", ...)
		// Determine the prototype to use (may differ from dateProto for cross-realm construction)
		instanceProto := vm.NewValueFromPlainObject(dateProto)
		if newTarget := vmInstance.GetNewTarget(); !newTarget.IsUndefined() {
			candidate, gpfcErr := vmInstance.GetPrototypeFromConstructor(newTarget, "%DatePrototype%")
			if gpfcErr != nil {
				return vm.Undefined, gpfcErr
			}
			if candidate.IsObject() {
				instanceProto = candidate
			}
		}

		// When called as constructor with 'new', create Date object
		var timestamp float64

		if len(args) == 0 {
			// new Date() - current time
			timestamp = float64(time.Now().UnixMilli())
		} else if len(args) == 1 {
			arg := args[0]
			if arg.IsNumber() {
				// new Date(timestamp)
				timestamp = arg.ToFloat()
			} else if arg.IsObject() {
				// Check if it's a Date object by looking for __timestamp__
				if ts, ok := getDateTimestamp(arg); ok {
					// new Date(dateObject) - copy constructor
					timestamp = ts
				} else {
					// Not a Date object, try string parsing
					dateStr := arg.ToString()
					if parsedTime, err := time.Parse(time.RFC3339, dateStr); err == nil {
						timestamp = float64(parsedTime.UnixMilli())
					} else if parsedTime, err := time.Parse("2006-01-02", dateStr); err == nil {
						// Date-only format per ECMAScript spec defaults to UTC
						timestamp = float64(parsedTime.UTC().UnixMilli())
					} else if parsedTime, err := time.Parse("2006", dateStr); err == nil {
						// Year-only format per ECMAScript spec defaults to January 1st UTC
						timestamp = float64(parsedTime.UTC().UnixMilli())
					} else {
						// Invalid date string - use NaN to indicate invalid date
						timestamp = float64(0x7FF8000000000000) // NaN value
					}
				}
			} else {
				// new Date(dateString) - simplified parsing
				dateStr := arg.ToString()
				if parsedTime, err := time.Parse(time.RFC3339, dateStr); err == nil {
					timestamp = float64(parsedTime.UnixMilli())
				} else if parsedTime, err := time.Parse("2006-01-02", dateStr); err == nil {
					// Date-only format per ECMAScript spec defaults to UTC
					timestamp = float64(parsedTime.UTC().UnixMilli())
				} else if parsedTime, err := time.Parse("2006", dateStr); err == nil {
					// Year-only format per ECMAScript spec defaults to January 1st UTC
					timestamp = float64(parsedTime.UTC().UnixMilli())
				} else {
					// Invalid date string - use NaN to indicate invalid date
					timestamp = math.NaN()
				}
			}
		} else {
			// new Date(year, month, day, ...)
			year := int(args[0].ToFloat())
			month := int(args[1].ToFloat()) + 1 // JavaScript months are 0-based
			day := 1
			hour := 0
			minute := 0
			second := 0
			nanosecond := 0

			if len(args) >= 3 {
				day = int(args[2].ToFloat())
			}
			if len(args) >= 4 {
				hour = int(args[3].ToFloat())
			}
			if len(args) >= 5 {
				minute = int(args[4].ToFloat())
			}
			if len(args) >= 6 {
				second = int(args[5].ToFloat())
			}
			if len(args) >= 7 {
				millisecond := int(args[6].ToFloat())
				nanosecond = millisecond * 1000000
			}

			t := time.Date(year, time.Month(month), day, hour, minute, second, nanosecond, time.Local)
			timestamp = float64(t.UnixMilli())
		}

		// Create Date object with timestamp stored as a property
		dateObj := vm.NewObject(instanceProto)
		dateObj.AsPlainObject().SetOwnNonEnumerable("__timestamp__", vm.NumberValue(timestamp))

		return dateObj, nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(dateProto))

	// Store Date prototype on VM for cross-realm support (GetPrototypeFromConstructor)
	vmInstance.DatePrototype = vm.NewValueFromPlainObject(dateProto)

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("now", vm.NewNativeFunction(0, false, "now", func(args []vm.Value) (vm.Value, error) {
		return vm.NumberValue(float64(time.Now().UnixMilli())), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("parse", vm.NewNativeFunction(1, false, "parse", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.NaN, nil
		}
		dateStr := args[0].ToString()

		// Try common date formats (UTC formats first per ECMAScript spec)
		// Year-only and date-only formats default to UTC
		if parsedTime, err := time.Parse(time.RFC3339, dateStr); err == nil {
			return vm.NumberValue(float64(parsedTime.UnixMilli())), nil
		}
		if parsedTime, err := time.Parse("2006-01-02T15:04:05Z", dateStr); err == nil {
			return vm.NumberValue(float64(parsedTime.UnixMilli())), nil
		}
		if parsedTime, err := time.Parse("2006-01-02", dateStr); err == nil {
			// Date-only per spec defaults to UTC
			return vm.NumberValue(float64(parsedTime.UTC().UnixMilli())), nil
		}
		if parsedTime, err := time.Parse("2006", dateStr); err == nil {
			// Year-only per spec defaults to January 1st UTC
			return vm.NumberValue(float64(parsedTime.UTC().UnixMilli())), nil
		}
		// Try other common formats (local time)
		otherFormats := []string{
			"01/02/2006",
			"January 2, 2006",
		}
		for _, format := range otherFormats {
			if parsedTime, err := time.Parse(format, dateStr); err == nil {
				return vm.NumberValue(float64(parsedTime.UnixMilli())), nil
			}
		}

		return vm.NaN, nil // Invalid date
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("UTC", vm.NewNativeFunction(2, true, "UTC", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.NaN, nil
		}

		year := int(args[0].ToFloat())
		month := int(args[1].ToFloat()) + 1 // JavaScript months are 0-based
		day := 1
		hour := 0
		minute := 0
		second := 0
		nanosecond := 0

		if len(args) >= 3 {
			day = int(args[2].ToFloat())
		}
		if len(args) >= 4 {
			hour = int(args[3].ToFloat())
		}
		if len(args) >= 5 {
			minute = int(args[4].ToFloat())
		}
		if len(args) >= 6 {
			second = int(args[5].ToFloat())
		}
		if len(args) >= 7 {
			millisecond := int(args[6].ToFloat())
			nanosecond = millisecond * 1000000
		}

		t := time.Date(year, time.Month(month), day, hour, minute, second, nanosecond, time.UTC)
		return vm.NumberValue(float64(t.UnixMilli())), nil
	}))

	dateCtor := ctorWithProps

	// Set constructor property on prototype
	dateProto.SetOwnNonEnumerable("constructor", dateCtor)

	// Add Symbol.toPrimitive method (ES6 20.3.4.45)
	// Date.prototype[@@toPrimitive](hint)
	if vmInstance.SymbolToPrimitive.Type() == vm.TypeSymbol {
		toPrimitiveFunc := vm.NewNativeFunction(1, false, "[Symbol.toPrimitive]", func(args []vm.Value) (vm.Value, error) {
			thisValue := vmInstance.GetThis()
			// Step 1: If Type(O) is not Object, throw a TypeError
			if !thisValue.IsObject() && thisValue.Type() != vm.TypeObject {
				return vm.Undefined, vmInstance.NewTypeError("Date.prototype[@@toPrimitive] requires that 'this' be an Object")
			}

			// Step 2: Get hint
			hint := "default"
			if len(args) > 0 {
				hint = args[0].ToString()
			}

			// Step 3: Validate hint
			if hint != "string" && hint != "number" && hint != "default" {
				return vm.Undefined, vmInstance.NewTypeError("Invalid hint: " + hint)
			}

			// Step 4: If hint is "default", let hint be "string"
			if hint == "default" {
				hint = "string"
			}

			// Step 5: Return ? OrdinaryToPrimitive(O, hint)
			// For "string": try toString first, then valueOf
			// For "number": try valueOf first, then toString
			if hint == "string" {
				// Try toString first
				if toStringMethod, err := vmInstance.GetProperty(thisValue, "toString"); err == nil && toStringMethod.IsCallable() {
					result, callErr := vmInstance.Call(toStringMethod, thisValue, nil)
					if callErr == nil && !result.IsObject() {
						return result, nil
					}
				}
				// Try valueOf
				if valueOfMethod, err := vmInstance.GetProperty(thisValue, "valueOf"); err == nil && valueOfMethod.IsCallable() {
					result, callErr := vmInstance.Call(valueOfMethod, thisValue, nil)
					if callErr == nil && !result.IsObject() {
						return result, nil
					}
				}
			} else {
				// hint == "number": try valueOf first
				if valueOfMethod, err := vmInstance.GetProperty(thisValue, "valueOf"); err == nil && valueOfMethod.IsCallable() {
					result, callErr := vmInstance.Call(valueOfMethod, thisValue, nil)
					if callErr == nil && !result.IsObject() {
						return result, nil
					}
				}
				// Try toString
				if toStringMethod, err := vmInstance.GetProperty(thisValue, "toString"); err == nil && toStringMethod.IsCallable() {
					result, callErr := vmInstance.Call(toStringMethod, thisValue, nil)
					if callErr == nil && !result.IsObject() {
						return result, nil
					}
				}
			}
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert object to primitive value")
		})
		// Set as non-enumerable, non-writable, configurable (per spec)
		wFalse, eFalse, cTrue := false, false, true
		dateProto.DefineOwnPropertyByKey(vm.NewSymbolKey(vmInstance.SymbolToPrimitive), toPrimitiveFunc, &wFalse, &eFalse, &cTrue)
	}

	// Set @@toStringTag to "Date" so Object.prototype.toString.call(new Date()) returns "[object Date]"
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal := false
		trueVal := true
		dateProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Date"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&trueVal,  // configurable: true
		)
	}

	// Register Date constructor as global
	return ctx.DefineGlobal("Date", dateCtor)
}

// Helper functions to get/set timestamp from Date objects

// thisTimeValue implements the abstract operation thisTimeValue(value) per ECMAScript spec.
// It returns the [[DateValue]] internal slot if value is a Date object, otherwise returns an error.
// This is used by Date.prototype methods to validate the "this" value.
func thisTimeValue(vmInstance *vm.VM, dateValue vm.Value) (float64, error) {
	obj := dateValue.AsPlainObject()
	if obj == nil {
		return 0, vmInstance.NewTypeError("this is not a Date object")
	}
	timestampValue, exists := obj.GetOwn("__timestamp__")
	if !exists {
		return 0, vmInstance.NewTypeError("this is not a Date object")
	}
	return timestampValue.ToFloat(), nil
}

// getDateTimestamp is a legacy helper that returns (timestamp, ok).
// For new code, prefer thisTimeValue which properly throws TypeError.
func getDateTimestamp(dateValue vm.Value) (float64, bool) {
	if obj := dateValue.AsPlainObject(); obj != nil {
		if timestampValue, exists := obj.GetOwn("__timestamp__"); exists {
			return timestampValue.ToFloat(), true
		}
	}
	return 0, false
}

// timeClip implements the ECMAScript TimeClip abstract operation (21.4.1.15)
// If |time| > 8.64 × 10^15, return NaN
func timeClip(t float64) float64 {
	if math.IsNaN(t) || math.IsInf(t, 0) || math.Abs(t) > 8.64e15 {
		return math.NaN()
	}
	return t
}

func setDateTimestamp(dateValue vm.Value, timestamp float64) float64 {
	clipped := timeClip(timestamp)
	if obj := dateValue.AsPlainObject(); obj != nil {
		obj.SetOwnNonEnumerable("__timestamp__", vm.NumberValue(clipped))
	}
	return clipped
}

package builtins

import (
	"fmt"
	"math"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"time"
)

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
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			return vm.NumberValue(timestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getFullYear", vm.NewNativeFunction(0, false, "getFullYear", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Year())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getMonth", vm.NewNativeFunction(0, false, "getMonth", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Month() - 1)), nil // JavaScript months are 0-based
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getDate", vm.NewNativeFunction(0, false, "getDate", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Day())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getDay", vm.NewNativeFunction(0, false, "getDay", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Weekday())), nil // Sunday = 0 in JavaScript
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getHours", vm.NewNativeFunction(0, false, "getHours", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Hour())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getMinutes", vm.NewNativeFunction(0, false, "getMinutes", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Minute())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getSeconds", vm.NewNativeFunction(0, false, "getSeconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Second())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getMilliseconds", vm.NewNativeFunction(0, false, "getMilliseconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Nanosecond() / 1000000)), nil
		}
		return vm.NaN, nil
	}))

	// UTC getter methods
	dateProto.SetOwnNonEnumerable("getUTCFullYear", vm.NewNativeFunction(0, false, "getUTCFullYear", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NumberValue(float64(t.Year())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCMonth", vm.NewNativeFunction(0, false, "getUTCMonth", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NumberValue(float64(t.Month() - 1)), nil // JavaScript months are 0-based
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCDate", vm.NewNativeFunction(0, false, "getUTCDate", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NumberValue(float64(t.Day())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCDay", vm.NewNativeFunction(0, false, "getUTCDay", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NumberValue(float64(t.Weekday())), nil // Sunday = 0 in JavaScript
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCHours", vm.NewNativeFunction(0, false, "getUTCHours", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NumberValue(float64(t.Hour())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCMinutes", vm.NewNativeFunction(0, false, "getUTCMinutes", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NumberValue(float64(t.Minute())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCSeconds", vm.NewNativeFunction(0, false, "getUTCSeconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NumberValue(float64(t.Second())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getUTCMilliseconds", vm.NewNativeFunction(0, false, "getUTCMilliseconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NumberValue(float64(t.Nanosecond() / 1000000)), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("getTimezoneOffset", vm.NewNativeFunction(0, false, "getTimezoneOffset", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			_, offset := t.Zone()
			return vm.NumberValue(float64(-offset / 60)), nil // JavaScript returns minutes, negative for east of UTC
		}
		return vm.NaN, nil
	}))

	// setTime method
	dateProto.SetOwnNonEnumerable("setTime", vm.NewNativeFunction(1, false, "setTime", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		newTimestamp := args[0].ToFloat()
		setDateTimestamp(thisDate, newTimestamp)
		return vm.NumberValue(newTimestamp), nil
	}))

	dateProto.SetOwnNonEnumerable("setDate", vm.NewNativeFunction(1, false, "setDate", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			day := int(args[0].ToFloat())
			newTime := time.Date(t.Year(), t.Month(), day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setMonth", vm.NewNativeFunction(2, false, "setMonth", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			month := time.Month(int(args[0].ToFloat()) + 1) // JavaScript months are 0-based
			
			// Use existing day if second parameter not provided
			day := t.Day()
			if len(args) >= 2 {
				day = int(args[1].ToFloat())
			}
			
			newTime := time.Date(t.Year(), month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setFullYear", vm.NewNativeFunction(3, false, "setFullYear", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			year := int(args[0].ToFloat())
			
			// Use existing month if second parameter not provided
			month := t.Month()
			if len(args) >= 2 {
				month = time.Month(int(args[1].ToFloat()) + 1) // JavaScript months are 0-based
			}
			
			// Use existing day if third parameter not provided
			day := t.Day()
			if len(args) >= 3 {
				day = int(args[2].ToFloat())
			}
			
			newTime := time.Date(year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setHours", vm.NewNativeFunction(4, false, "setHours", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			hour := int(args[0].ToFloat())
			
			// Use existing values if additional parameters not provided
			minute := t.Minute()
			if len(args) >= 2 {
				minute = int(args[1].ToFloat())
			}
			
			second := t.Second()
			if len(args) >= 3 {
				second = int(args[2].ToFloat())
			}
			
			nanosecond := t.Nanosecond()
			if len(args) >= 4 {
				millisecond := int(args[3].ToFloat())
				nanosecond = millisecond * 1000000
			}
			
			newTime := time.Date(t.Year(), t.Month(), t.Day(), hour, minute, second, nanosecond, t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setMinutes", vm.NewNativeFunction(3, false, "setMinutes", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			minute := int(args[0].ToFloat())
			
			// Use existing values if additional parameters not provided
			second := t.Second()
			if len(args) >= 2 {
				second = int(args[1].ToFloat())
			}
			
			nanosecond := t.Nanosecond()
			if len(args) >= 3 {
				millisecond := int(args[2].ToFloat())
				nanosecond = millisecond * 1000000
			}
			
			newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, second, nanosecond, t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setSeconds", vm.NewNativeFunction(2, false, "setSeconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			second := int(args[0].ToFloat())
			
			// Use existing value if additional parameter not provided
			nanosecond := t.Nanosecond()
			if len(args) >= 2 {
				millisecond := int(args[1].ToFloat())
				nanosecond = millisecond * 1000000
			}
			
			newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), second, nanosecond, t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setMilliseconds", vm.NewNativeFunction(1, false, "setMilliseconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			millisecond := int(args[0].ToFloat()) * 1000000 // Convert ms to ns
			newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), millisecond, t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NewString(t.Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwnNonEnumerable("toISOString", vm.NewNativeFunction(0, false, "toISOString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NewString(t.Format("2006-01-02T15:04:05.000Z")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwnNonEnumerable("toDateString", vm.NewNativeFunction(0, false, "toDateString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NewString(t.Format("Mon Jan 02 2006")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwnNonEnumerable("toTimeString", vm.NewNativeFunction(0, false, "toTimeString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NewString(t.Format("15:04:05 GMT-0700 (MST)")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			return vm.NumberValue(timestamp), nil
		}
		return vm.NaN, nil
	}))

	// Locale methods
	dateProto.SetOwnNonEnumerable("toLocaleString", vm.NewNativeFunction(0, false, "toLocaleString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			// Simple locale format - could be enhanced with actual locale support
			return vm.NewString(t.Format("1/2/2006, 3:04:05 PM")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwnNonEnumerable("toLocaleDateString", vm.NewNativeFunction(0, false, "toLocaleDateString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			// Simple locale format - could be enhanced with actual locale support
			return vm.NewString(t.Format("1/2/2006")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwnNonEnumerable("toLocaleTimeString", vm.NewNativeFunction(0, false, "toLocaleTimeString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			// Simple locale format - could be enhanced with actual locale support
			return vm.NewString(t.Format("3:04:05 PM")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	// toJSON method
	dateProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			// Check if the timestamp is NaN (invalid date)
			if math.IsNaN(timestamp) {
				return vm.Null, nil
			}
			// toJSON returns the same as toISOString
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NewString(t.Format("2006-01-02T15:04:05.000Z")), nil
		}
		// For invalid dates, toJSON returns null
		return vm.Null, nil
	}))

	// Annex B methods
	dateProto.SetOwnNonEnumerable("getYear", vm.NewNativeFunction(0, false, "getYear", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			// getYear returns year - 1900
			return vm.NumberValue(float64(t.Year() - 1900)), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setYear", vm.NewNativeFunction(1, false, "setYear", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		dateObj := thisDate.AsPlainObject()
		if dateObj == nil {
			return vm.Undefined, fmt.Errorf("setYear called on non-Date object")
		}

		// Check if it's a Date object by verifying it has __timestamp__ property
		if _, exists := dateObj.GetOwn("__timestamp__"); !exists {
			return vm.Undefined, fmt.Errorf("setYear called on non-Date object")
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
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			year := int(args[0].ToFloat())
			
			// Use existing month if second parameter not provided
			month := t.Month()
			if len(args) >= 2 {
				month = time.Month(int(args[1].ToFloat()) + 1) // JavaScript months are 0-based
			}
			
			// Use existing day if third parameter not provided
			day := t.Day()
			if len(args) >= 3 {
				day = int(args[2].ToFloat())
			}
			
			newTime := time.Date(year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCMonth", vm.NewNativeFunction(2, false, "setUTCMonth", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			month := time.Month(int(args[0].ToFloat()) + 1) // JavaScript months are 0-based
			
			// Use existing day if second parameter not provided
			day := t.Day()
			if len(args) >= 2 {
				day = int(args[1].ToFloat())
			}
			
			newTime := time.Date(t.Year(), month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCDate", vm.NewNativeFunction(1, false, "setUTCDate", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			day := int(args[0].ToFloat())
			newTime := time.Date(t.Year(), t.Month(), day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCHours", vm.NewNativeFunction(4, false, "setUTCHours", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			hour := int(args[0].ToFloat())
			
			// Use existing values if additional parameters not provided
			minute := t.Minute()
			if len(args) >= 2 {
				minute = int(args[1].ToFloat())
			}
			
			second := t.Second()
			if len(args) >= 3 {
				second = int(args[2].ToFloat())
			}
			
			nanosecond := t.Nanosecond()
			if len(args) >= 4 {
				millisecond := int(args[3].ToFloat())
				nanosecond = millisecond * 1000000
			}
			
			newTime := time.Date(t.Year(), t.Month(), t.Day(), hour, minute, second, nanosecond, time.UTC)
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCMinutes", vm.NewNativeFunction(3, false, "setUTCMinutes", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			minute := int(args[0].ToFloat())
			
			// Use existing values if additional parameters not provided
			second := t.Second()
			if len(args) >= 2 {
				second = int(args[1].ToFloat())
			}
			
			nanosecond := t.Nanosecond()
			if len(args) >= 3 {
				millisecond := int(args[2].ToFloat())
				nanosecond = millisecond * 1000000
			}
			
			newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, second, nanosecond, time.UTC)
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCSeconds", vm.NewNativeFunction(2, false, "setUTCSeconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			second := int(args[0].ToFloat())
			
			// Use existing value if additional parameter not provided
			nanosecond := t.Nanosecond()
			if len(args) >= 2 {
				millisecond := int(args[1].ToFloat())
				nanosecond = millisecond * 1000000
			}
			
			newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), second, nanosecond, time.UTC)
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwnNonEnumerable("setUTCMilliseconds", vm.NewNativeFunction(1, false, "setUTCMilliseconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			millisecond := int(args[0].ToFloat()) * 1000000 // Convert ms to ns
			newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), millisecond, time.UTC)
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	// Create Date constructor
	ctorWithProps := vm.NewNativeFunctionWithProps(-1, true, "Date", func(args []vm.Value) (vm.Value, error) {
		// When called as a function (not constructor), return current date/time string
		if !vmInstance.IsConstructorCall() {
			t := time.Now()
			return vm.NewString(t.Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")), nil
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
						timestamp = float64(parsedTime.UnixMilli())
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
					timestamp = float64(parsedTime.UnixMilli())
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
		dateObj := vm.NewObject(vm.NewValueFromPlainObject(dateProto))
		dateObj.AsPlainObject().SetOwnNonEnumerable("__timestamp__", vm.NumberValue(timestamp))
		
		return dateObj, nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(dateProto))

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("now", vm.NewNativeFunction(0, false, "now", func(args []vm.Value) (vm.Value, error) {
		return vm.NumberValue(float64(time.Now().UnixMilli())), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("parse", vm.NewNativeFunction(1, false, "parse", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.NaN, nil
		}
		dateStr := args[0].ToString()

		// Try common date formats
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z",
			"2006-01-02",
			"01/02/2006",
			"January 2, 2006",
		}

		for _, format := range formats {
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

	// Set Date prototype in VM (if needed)
	// vmInstance.DatePrototype = vm.NewValueFromPlainObject(dateProto)

	// Register Date constructor as global
	return ctx.DefineGlobal("Date", dateCtor)
}

// Helper functions to get/set timestamp from Date objects
func getDateTimestamp(dateValue vm.Value) (float64, bool) {
	if obj := dateValue.AsPlainObject(); obj != nil {
		if timestampValue, exists := obj.GetOwn("__timestamp__"); exists {
			return timestampValue.ToFloat(), true
		}
	}
	return 0, false
}

func setDateTimestamp(dateValue vm.Value, timestamp float64) {
	if obj := dateValue.AsPlainObject(); obj != nil {
		obj.SetOwnNonEnumerable("__timestamp__", vm.NumberValue(timestamp))
	}
}

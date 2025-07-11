package builtins

import (
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
		WithProperty("setFullYear", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number}, types.Number, []bool{false, true, true})).
		WithProperty("setMonth", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Number, []bool{false, true})).
		WithProperty("setDate", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("setHours", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("setMinutes", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("setSeconds", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("setMilliseconds", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toISOString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toDateString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toTimeString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("constructor", types.Any) // Avoid circular reference, use Any for constructor property

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
	dateProto.SetOwn("getTime", vm.NewNativeFunction(0, false, "getTime", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			return vm.NumberValue(timestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("getFullYear", vm.NewNativeFunction(0, false, "getFullYear", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Year())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("getMonth", vm.NewNativeFunction(0, false, "getMonth", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Month() - 1)), nil // JavaScript months are 0-based
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("getDate", vm.NewNativeFunction(0, false, "getDate", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Day())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("getDay", vm.NewNativeFunction(0, false, "getDay", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Weekday())), nil // Sunday = 0 in JavaScript
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("getHours", vm.NewNativeFunction(0, false, "getHours", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Hour())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("getMinutes", vm.NewNativeFunction(0, false, "getMinutes", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Minute())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("getSeconds", vm.NewNativeFunction(0, false, "getSeconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Second())), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("getMilliseconds", vm.NewNativeFunction(0, false, "getMilliseconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NumberValue(float64(t.Nanosecond() / 1000000)), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("setDate", vm.NewNativeFunction(1, false, "setDate", func(args []vm.Value) (vm.Value, error) {
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

	dateProto.SetOwn("setMonth", vm.NewNativeFunction(2, false, "setMonth", func(args []vm.Value) (vm.Value, error) {
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

	dateProto.SetOwn("setFullYear", vm.NewNativeFunction(3, false, "setFullYear", func(args []vm.Value) (vm.Value, error) {
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

	dateProto.SetOwn("setHours", vm.NewNativeFunction(1, false, "setHours", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			hour := int(args[0].ToFloat())
			newTime := time.Date(t.Year(), t.Month(), t.Day(), hour, t.Minute(), t.Second(), t.Nanosecond(), t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("setMinutes", vm.NewNativeFunction(1, false, "setMinutes", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			minute := int(args[0].ToFloat())
			newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, t.Second(), t.Nanosecond(), t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("setSeconds", vm.NewNativeFunction(1, false, "setSeconds", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if len(args) < 1 {
			return vm.NaN, nil
		}
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			second := int(args[0].ToFloat())
			newTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), second, t.Nanosecond(), t.Location())
			newTimestamp := float64(newTime.UnixMilli())
			setDateTimestamp(thisDate, newTimestamp)
			return vm.NumberValue(newTimestamp), nil
		}
		return vm.NaN, nil
	}))

	dateProto.SetOwn("setMilliseconds", vm.NewNativeFunction(1, false, "setMilliseconds", func(args []vm.Value) (vm.Value, error) {
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

	dateProto.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NewString(t.Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwn("toISOString", vm.NewNativeFunction(0, false, "toISOString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp)).UTC()
			return vm.NewString(t.Format("2006-01-02T15:04:05.000Z")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwn("toDateString", vm.NewNativeFunction(0, false, "toDateString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NewString(t.Format("Mon Jan 02 2006")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwn("toTimeString", vm.NewNativeFunction(0, false, "toTimeString", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			t := time.UnixMilli(int64(timestamp))
			return vm.NewString(t.Format("15:04:05 GMT-0700 (MST)")), nil
		}
		return vm.NewString("Invalid Date"), nil
	}))

	dateProto.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		thisDate := vmInstance.GetThis()
		if timestamp, ok := getDateTimestamp(thisDate); ok {
			return vm.NumberValue(timestamp), nil
		}
		return vm.NaN, nil
	}))

	// Create Date constructor
	ctorWithProps := vm.NewNativeFunctionWithProps(-1, true, "Date", func(args []vm.Value) (vm.Value, error) {
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
						timestamp = float64(time.Now().UnixMilli()) // fallback
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
					timestamp = float64(time.Now().UnixMilli()) // fallback
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

			t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
			timestamp = float64(t.UnixMilli())
		}

		// Create Date object with timestamp stored as a property
		dateObj := vm.NewObject(vm.NewValueFromPlainObject(dateProto))
		dateObj.AsPlainObject().SetOwn("__timestamp__", vm.NumberValue(timestamp))
		
		return dateObj, nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vm.NewValueFromPlainObject(dateProto))

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("now", vm.NewNativeFunction(0, false, "now", func(args []vm.Value) (vm.Value, error) {
		return vm.NumberValue(float64(time.Now().UnixMilli())), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("parse", vm.NewNativeFunction(1, false, "parse", func(args []vm.Value) (vm.Value, error) {
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

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("UTC", vm.NewNativeFunction(2, true, "UTC", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.NaN, nil
		}

		year := int(args[0].ToFloat())
		month := int(args[1].ToFloat()) + 1 // JavaScript months are 0-based
		day := 1
		hour := 0
		minute := 0
		second := 0

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

		t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
		return vm.NumberValue(float64(t.UnixMilli())), nil
	}))

	dateCtor := ctorWithProps

	// Set constructor property on prototype
	dateProto.SetOwn("constructor", dateCtor)

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
		obj.SetOwn("__timestamp__", vm.NumberValue(timestamp))
	}
}

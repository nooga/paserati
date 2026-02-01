package builtins

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

const PriorityTemporal = 105 // After Date

type TemporalInitializer struct{}

func (t *TemporalInitializer) Name() string {
	return "Temporal"
}

func (t *TemporalInitializer) Priority() int {
	return PriorityTemporal
}

func (t *TemporalInitializer) InitTypes(ctx *TypeContext) error {
	// Create Temporal.Now type
	temporalNowType := types.NewObjectType().
		WithProperty("timeZoneId", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("instant", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("plainDateISO", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("plainTimeISO", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("plainDateTimeISO", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("zonedDateTimeISO", types.NewSimpleFunction([]types.Type{}, types.Any))

	// Create Temporal.Instant type
	temporalInstantType := types.NewObjectType().
		WithProperty("epochSeconds", types.Number).
		WithProperty("epochMilliseconds", types.Number).
		WithProperty("epochMicroseconds", types.BigInt).
		WithProperty("epochNanoseconds", types.BigInt)

	// Create Temporal namespace type
	temporalType := types.NewObjectType().
		WithProperty("Now", temporalNowType).
		WithProperty("Instant", temporalInstantType)

	// Define Temporal namespace in global environment
	return ctx.DefineGlobal("Temporal", temporalType)
}

func (t *TemporalInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM
	falseVal := false
	trueVal := true

	// ============================================
	// Create Temporal namespace object
	// ============================================
	temporalObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Set @@toStringTag to "Temporal"
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		temporalObj.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&trueVal,  // configurable: true
		)
	}

	// ============================================
	// Create Temporal.Instant.prototype
	// ============================================
	instantProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Set @@toStringTag to "Temporal.Instant"
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		instantProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal.Instant"),
			&falseVal,
			&falseVal,
			&trueVal,
		)
	}

	// Helper to get nanoseconds from a Temporal.Instant object
	getInstantNanos := func(val vm.Value) (*big.Int, error) {
		if !val.IsObject() {
			return nil, vmInstance.NewTypeError("Value is not a Temporal.Instant")
		}
		obj := val.AsPlainObject()
		if obj == nil {
			return nil, vmInstance.NewTypeError("Value is not a Temporal.Instant")
		}
		nanosVal, exists := obj.GetOwn("[[EpochNanoseconds]]")
		if !exists {
			return nil, vmInstance.NewTypeError("Value is not a Temporal.Instant")
		}
		if nanosVal.Type() != vm.TypeBigInt {
			return nil, vmInstance.NewTypeError("Invalid Temporal.Instant internal state")
		}
		return nanosVal.AsBigInt(), nil
	}

	// Helper to create a new Temporal.Instant from nanoseconds
	createInstant := func(nanos *big.Int) vm.Value {
		instant := vm.NewObject(vm.NewValueFromPlainObject(instantProto)).AsPlainObject()
		instant.SetOwn("[[EpochNanoseconds]]", vm.NewBigInt(nanos))
		return vm.NewValueFromPlainObject(instant)
	}

	// Temporal.Instant.prototype.epochSeconds (getter)
	epochSecondsGetter := vm.NewNativeFunction(0, false, "get epochSeconds", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		nanos, err := getInstantNanos(thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// Convert nanoseconds to seconds (integer division)
		billion := big.NewInt(1_000_000_000)
		seconds := new(big.Int).Div(nanos, billion)
		// Return as number (may lose precision for very large values)
		return vm.NumberValue(float64(seconds.Int64())), nil
	})
	instantProto.DefineAccessorProperty("epochSeconds", epochSecondsGetter, true, vm.Undefined, false, &falseVal, &trueVal)

	// Temporal.Instant.prototype.epochMilliseconds (getter)
	epochMillisecondsGetter := vm.NewNativeFunction(0, false, "get epochMilliseconds", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		nanos, err := getInstantNanos(thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		million := big.NewInt(1_000_000)
		millis := new(big.Int).Div(nanos, million)
		return vm.NumberValue(float64(millis.Int64())), nil
	})
	instantProto.DefineAccessorProperty("epochMilliseconds", epochMillisecondsGetter, true, vm.Undefined, false, &falseVal, &trueVal)

	// Temporal.Instant.prototype.epochMicroseconds (getter) - returns BigInt
	epochMicrosecondsGetter := vm.NewNativeFunction(0, false, "get epochMicroseconds", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		nanos, err := getInstantNanos(thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		thousand := big.NewInt(1_000)
		micros := new(big.Int).Div(nanos, thousand)
		return vm.NewBigInt(micros), nil
	})
	instantProto.DefineAccessorProperty("epochMicroseconds", epochMicrosecondsGetter, true, vm.Undefined, false, &falseVal, &trueVal)

	// Temporal.Instant.prototype.epochNanoseconds (getter) - returns BigInt
	epochNanosecondsGetter := vm.NewNativeFunction(0, false, "get epochNanoseconds", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		nanos, err := getInstantNanos(thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewBigInt(new(big.Int).Set(nanos)), nil
	})
	instantProto.DefineAccessorProperty("epochNanoseconds", epochNanosecondsGetter, true, vm.Undefined, false, &falseVal, &trueVal)

	// Temporal.Instant.prototype.toString()
	instantProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		nanos, err := getInstantNanos(thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// Convert to time.Time for formatting
		sec := new(big.Int).Div(nanos, big.NewInt(1_000_000_000))
		nsec := new(big.Int).Mod(nanos, big.NewInt(1_000_000_000))
		t := time.Unix(sec.Int64(), nsec.Int64()).UTC()
		// Format as ISO 8601 with Z suffix
		// Temporal uses variable precision for subseconds
		formatted := t.Format("2006-01-02T15:04:05")
		nano := t.Nanosecond()
		if nano > 0 {
			// Add fractional seconds, trimming trailing zeros
			fracStr := fmt.Sprintf(".%09d", nano)
			fracStr = strings.TrimRight(fracStr, "0")
			formatted += fracStr
		}
		formatted += "Z"
		return vm.NewString(formatted), nil
	}))

	// Temporal.Instant.prototype.toJSON()
	instantProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		nanos, err := getInstantNanos(thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		sec := new(big.Int).Div(nanos, big.NewInt(1_000_000_000))
		nsec := new(big.Int).Mod(nanos, big.NewInt(1_000_000_000))
		t := time.Unix(sec.Int64(), nsec.Int64()).UTC()
		formatted := t.Format("2006-01-02T15:04:05")
		nano := t.Nanosecond()
		if nano > 0 {
			fracStr := fmt.Sprintf(".%09d", nano)
			fracStr = strings.TrimRight(fracStr, "0")
			formatted += fracStr
		}
		formatted += "Z"
		return vm.NewString(formatted), nil
	}))

	// Temporal.Instant.prototype.valueOf() - throws TypeError per spec
	instantProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("use compare() or equals() to compare Temporal.Instant")
	}))

	// Temporal.Instant.prototype.equals(other)
	instantProto.SetOwnNonEnumerable("equals", vm.NewNativeFunction(1, false, "equals", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		thisNanos, err := getInstantNanos(thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("equals requires an argument")
		}
		otherNanos, err := getInstantNanos(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(thisNanos.Cmp(otherNanos) == 0), nil
	}))

	// ============================================
	// Create Temporal.Instant constructor
	// ============================================
	instantCtor := vm.NewConstructorWithProps(1, false, "Instant", func(args []vm.Value) (vm.Value, error) {
		// Check if called as constructor
		if !vmInstance.IsConstructorCall() {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.Instant must be called with new")
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.Instant requires epochNanoseconds argument")
		}
		// First argument must be a BigInt
		epochNanos := args[0]
		if epochNanos.Type() != vm.TypeBigInt {
			return vm.Undefined, vmInstance.NewTypeError("epochNanoseconds must be a BigInt")
		}
		nanos := epochNanos.AsBigInt()

		// Validate range: must be within ±10^17 seconds from Unix epoch
		// That's ±10^26 nanoseconds
		maxNanos := new(big.Int).Exp(big.NewInt(10), big.NewInt(26), nil)
		minNanos := new(big.Int).Neg(maxNanos)
		if nanos.Cmp(maxNanos) > 0 || nanos.Cmp(minNanos) < 0 {
			return vm.Undefined, vmInstance.NewRangeError("epochNanoseconds out of range")
		}

		return createInstant(nanos), nil
	})

	// Set prototype property on constructor
	instantCtorProps := instantCtor.AsNativeFunctionWithProps()
	w, e, c := false, false, false
	instantCtorProps.Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(instantProto), &w, &e, &c)

	// Temporal.Instant.from(item) - static method
	instantCtorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.Instant.from requires an argument")
		}
		item := args[0]

		// If already a Temporal.Instant, return a copy
		if nanos, err := getInstantNanos(item); err == nil {
			return createInstant(new(big.Int).Set(nanos)), nil
		}

		// If string, parse ISO 8601
		if item.Type() == vm.TypeString {
			str := item.ToString()
			return parseInstantString(vmInstance, str, createInstant)
		}

		return vm.Undefined, vmInstance.NewTypeError("Invalid argument for Temporal.Instant.from")
	}))

	// Temporal.Instant.fromEpochSeconds(epochSeconds)
	instantCtorProps.Properties.SetOwnNonEnumerable("fromEpochSeconds", vm.NewNativeFunction(1, false, "fromEpochSeconds", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("fromEpochSeconds requires an argument")
		}
		seconds := args[0].ToFloat()
		if seconds != seconds { // NaN check
			return vm.Undefined, vmInstance.NewRangeError("Invalid epoch seconds")
		}
		nanos := new(big.Int).SetInt64(int64(seconds))
		nanos.Mul(nanos, big.NewInt(1_000_000_000))
		return createInstant(nanos), nil
	}))

	// Temporal.Instant.fromEpochMilliseconds(epochMilliseconds)
	instantCtorProps.Properties.SetOwnNonEnumerable("fromEpochMilliseconds", vm.NewNativeFunction(1, false, "fromEpochMilliseconds", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("fromEpochMilliseconds requires an argument")
		}
		millis := args[0].ToFloat()
		if millis != millis { // NaN check
			return vm.Undefined, vmInstance.NewRangeError("Invalid epoch milliseconds")
		}
		nanos := new(big.Int).SetInt64(int64(millis))
		nanos.Mul(nanos, big.NewInt(1_000_000))
		return createInstant(nanos), nil
	}))

	// Temporal.Instant.fromEpochMicroseconds(epochMicroseconds) - takes BigInt
	instantCtorProps.Properties.SetOwnNonEnumerable("fromEpochMicroseconds", vm.NewNativeFunction(1, false, "fromEpochMicroseconds", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("fromEpochMicroseconds requires an argument")
		}
		if args[0].Type() != vm.TypeBigInt {
			return vm.Undefined, vmInstance.NewTypeError("epochMicroseconds must be a BigInt")
		}
		micros := args[0].AsBigInt()
		nanos := new(big.Int).Mul(micros, big.NewInt(1_000))
		return createInstant(nanos), nil
	}))

	// Temporal.Instant.fromEpochNanoseconds(epochNanoseconds) - takes BigInt
	instantCtorProps.Properties.SetOwnNonEnumerable("fromEpochNanoseconds", vm.NewNativeFunction(1, false, "fromEpochNanoseconds", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("fromEpochNanoseconds requires an argument")
		}
		if args[0].Type() != vm.TypeBigInt {
			return vm.Undefined, vmInstance.NewTypeError("epochNanoseconds must be a BigInt")
		}
		nanos := args[0].AsBigInt()
		return createInstant(new(big.Int).Set(nanos)), nil
	}))

	// Temporal.Instant.compare(one, two)
	instantCtorProps.Properties.SetOwnNonEnumerable("compare", vm.NewNativeFunction(2, false, "compare", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("compare requires two arguments")
		}
		nanos1, err := getInstantNanos(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		nanos2, err := getInstantNanos(args[1])
		if err != nil {
			return vm.Undefined, err
		}
		cmp := nanos1.Cmp(nanos2)
		return vm.NumberValue(float64(cmp)), nil
	}))

	// Set constructor property on prototype
	instantProto.SetOwnNonEnumerable("constructor", instantCtor)

	// Add Temporal.Instant to namespace
	temporalObj.SetOwnNonEnumerable("Instant", instantCtor)

	// ============================================
	// Create Temporal.PlainDate.prototype
	// ============================================
	plainDateProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		plainDateProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal.PlainDate"),
			&falseVal, &falseVal, &trueVal,
		)
	}

	// Helper to get date components from PlainDate
	getPlainDateFields := func(val vm.Value) (year, month, day int, err error) {
		if !val.IsObject() {
			return 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainDate")
		}
		obj := val.AsPlainObject()
		if obj == nil {
			return 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainDate")
		}
		yVal, yOk := obj.GetOwn("[[ISOYear]]")
		mVal, mOk := obj.GetOwn("[[ISOMonth]]")
		dVal, dOk := obj.GetOwn("[[ISODay]]")
		if !yOk || !mOk || !dOk {
			return 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainDate")
		}
		return int(yVal.ToFloat()), int(mVal.ToFloat()), int(dVal.ToFloat()), nil
	}

	// Helper to create PlainDate
	createPlainDate := func(year, month, day int) vm.Value {
		date := vm.NewObject(vm.NewValueFromPlainObject(plainDateProto)).AsPlainObject()
		date.SetOwn("[[ISOYear]]", vm.NumberValue(float64(year)))
		date.SetOwn("[[ISOMonth]]", vm.NumberValue(float64(month)))
		date.SetOwn("[[ISODay]]", vm.NumberValue(float64(day)))
		date.SetOwn("[[Calendar]]", vm.NewString("iso8601"))
		return vm.NewValueFromPlainObject(date)
	}

	// PlainDate getters
	plainDateProto.DefineAccessorProperty("year", vm.NewNativeFunction(0, false, "get year", func(args []vm.Value) (vm.Value, error) {
		y, _, _, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(y)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateProto.DefineAccessorProperty("month", vm.NewNativeFunction(0, false, "get month", func(args []vm.Value) (vm.Value, error) {
		_, m, _, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(m)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateProto.DefineAccessorProperty("day", vm.NewNativeFunction(0, false, "get day", func(args []vm.Value) (vm.Value, error) {
		_, _, d, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(d)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateProto.DefineAccessorProperty("calendarId", vm.NewNativeFunction(0, false, "get calendarId", func(args []vm.Value) (vm.Value, error) {
		return vm.NewString("iso8601"), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateProto.DefineAccessorProperty("dayOfWeek", vm.NewNativeFunction(0, false, "get dayOfWeek", func(args []vm.Value) (vm.Value, error) {
		y, m, d, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		dow := int(t.Weekday())
		if dow == 0 {
			dow = 7 // Sunday is 7 in ISO
		}
		return vm.NumberValue(float64(dow)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateProto.DefineAccessorProperty("dayOfYear", vm.NewNativeFunction(0, false, "get dayOfYear", func(args []vm.Value) (vm.Value, error) {
		y, m, d, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		return vm.NumberValue(float64(t.YearDay())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateProto.DefineAccessorProperty("daysInMonth", vm.NewNativeFunction(0, false, "get daysInMonth", func(args []vm.Value) (vm.Value, error) {
		y, m, _, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		// Days in month: go to first of next month, subtract 1 day
		nextMonth := time.Date(y, time.Month(m)+1, 1, 0, 0, 0, 0, time.UTC)
		lastDay := nextMonth.AddDate(0, 0, -1)
		return vm.NumberValue(float64(lastDay.Day())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateProto.DefineAccessorProperty("daysInYear", vm.NewNativeFunction(0, false, "get daysInYear", func(args []vm.Value) (vm.Value, error) {
		y, _, _, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if isLeapYear(y) {
			return vm.NumberValue(366), nil
		}
		return vm.NumberValue(365), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateProto.DefineAccessorProperty("inLeapYear", vm.NewNativeFunction(0, false, "get inLeapYear", func(args []vm.Value) (vm.Value, error) {
		y, _, _, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(isLeapYear(y)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		y, m, d, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(fmt.Sprintf("%04d-%02d-%02d", y, m, d)), nil
	}))

	plainDateProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		y, m, d, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(fmt.Sprintf("%04d-%02d-%02d", y, m, d)), nil
	}))

	plainDateProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("use compare() to compare Temporal.PlainDate")
	}))

	// PlainDate constructor
	plainDateCtor := vm.NewConstructorWithProps(3, false, "PlainDate", func(args []vm.Value) (vm.Value, error) {
		if !vmInstance.IsConstructorCall() {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainDate must be called with new")
		}
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainDate requires year, month, day arguments")
		}
		year := int(args[0].ToFloat())
		month := int(args[1].ToFloat())
		day := int(args[2].ToFloat())
		// Basic validation
		if month < 1 || month > 12 {
			return vm.Undefined, vmInstance.NewRangeError("Invalid month")
		}
		if day < 1 || day > 31 {
			return vm.Undefined, vmInstance.NewRangeError("Invalid day")
		}
		return createPlainDate(year, month, day), nil
	})
	plainDateCtorProps := plainDateCtor.AsNativeFunctionWithProps()
	plainDateCtorProps.Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(plainDateProto), &w, &e, &c)

	plainDateCtorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainDate.from requires an argument")
		}
		item := args[0]
		if y, m, d, err := getPlainDateFields(item); err == nil {
			return createPlainDate(y, m, d), nil
		}
		if item.Type() == vm.TypeString {
			return parsePlainDateString(vmInstance, item.ToString(), createPlainDate)
		}
		return vm.Undefined, vmInstance.NewTypeError("Invalid argument for Temporal.PlainDate.from")
	}))

	plainDateCtorProps.Properties.SetOwnNonEnumerable("compare", vm.NewNativeFunction(2, false, "compare", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("compare requires two arguments")
		}
		y1, m1, d1, err := getPlainDateFields(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		y2, m2, d2, err := getPlainDateFields(args[1])
		if err != nil {
			return vm.Undefined, err
		}
		if y1 != y2 {
			if y1 < y2 {
				return vm.NumberValue(-1), nil
			}
			return vm.NumberValue(1), nil
		}
		if m1 != m2 {
			if m1 < m2 {
				return vm.NumberValue(-1), nil
			}
			return vm.NumberValue(1), nil
		}
		if d1 != d2 {
			if d1 < d2 {
				return vm.NumberValue(-1), nil
			}
			return vm.NumberValue(1), nil
		}
		return vm.NumberValue(0), nil
	}))

	plainDateProto.SetOwnNonEnumerable("constructor", plainDateCtor)
	temporalObj.SetOwnNonEnumerable("PlainDate", plainDateCtor)

	// ============================================
	// Create Temporal.PlainTime.prototype
	// ============================================
	plainTimeProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		plainTimeProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal.PlainTime"),
			&falseVal, &falseVal, &trueVal,
		)
	}

	getPlainTimeFields := func(val vm.Value) (hour, minute, second, ms, us, ns int, err error) {
		if !val.IsObject() {
			return 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainTime")
		}
		obj := val.AsPlainObject()
		if obj == nil {
			return 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainTime")
		}
		hVal, hOk := obj.GetOwn("[[ISOHour]]")
		minVal, minOk := obj.GetOwn("[[ISOMinute]]")
		sVal, sOk := obj.GetOwn("[[ISOSecond]]")
		msVal, msOk := obj.GetOwn("[[ISOMillisecond]]")
		usVal, usOk := obj.GetOwn("[[ISOMicrosecond]]")
		nsVal, nsOk := obj.GetOwn("[[ISONanosecond]]")
		if !hOk || !minOk || !sOk || !msOk || !usOk || !nsOk {
			return 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainTime")
		}
		return int(hVal.ToFloat()), int(minVal.ToFloat()), int(sVal.ToFloat()), int(msVal.ToFloat()), int(usVal.ToFloat()), int(nsVal.ToFloat()), nil
	}

	createPlainTime := func(hour, minute, second, ms, us, ns int) vm.Value {
		t := vm.NewObject(vm.NewValueFromPlainObject(plainTimeProto)).AsPlainObject()
		t.SetOwn("[[ISOHour]]", vm.NumberValue(float64(hour)))
		t.SetOwn("[[ISOMinute]]", vm.NumberValue(float64(minute)))
		t.SetOwn("[[ISOSecond]]", vm.NumberValue(float64(second)))
		t.SetOwn("[[ISOMillisecond]]", vm.NumberValue(float64(ms)))
		t.SetOwn("[[ISOMicrosecond]]", vm.NumberValue(float64(us)))
		t.SetOwn("[[ISONanosecond]]", vm.NumberValue(float64(ns)))
		return vm.NewValueFromPlainObject(t)
	}

	// PlainTime getters
	plainTimeProto.DefineAccessorProperty("hour", vm.NewNativeFunction(0, false, "get hour", func(args []vm.Value) (vm.Value, error) {
		h, _, _, _, _, _, err := getPlainTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(h)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainTimeProto.DefineAccessorProperty("minute", vm.NewNativeFunction(0, false, "get minute", func(args []vm.Value) (vm.Value, error) {
		_, m, _, _, _, _, err := getPlainTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(m)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainTimeProto.DefineAccessorProperty("second", vm.NewNativeFunction(0, false, "get second", func(args []vm.Value) (vm.Value, error) {
		_, _, s, _, _, _, err := getPlainTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(s)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainTimeProto.DefineAccessorProperty("millisecond", vm.NewNativeFunction(0, false, "get millisecond", func(args []vm.Value) (vm.Value, error) {
		_, _, _, ms, _, _, err := getPlainTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(ms)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainTimeProto.DefineAccessorProperty("microsecond", vm.NewNativeFunction(0, false, "get microsecond", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, us, _, err := getPlainTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(us)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainTimeProto.DefineAccessorProperty("nanosecond", vm.NewNativeFunction(0, false, "get nanosecond", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, ns, err := getPlainTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(ns)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainTimeProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		h, m, s, ms, us, ns, err := getPlainTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		result := fmt.Sprintf("%02d:%02d:%02d", h, m, s)
		// Add subsecond precision if needed
		totalNano := ms*1000000 + us*1000 + ns
		if totalNano > 0 {
			fracStr := fmt.Sprintf(".%09d", totalNano)
			fracStr = strings.TrimRight(fracStr, "0")
			result += fracStr
		}
		return vm.NewString(result), nil
	}))

	plainTimeProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		h, m, s, ms, us, ns, err := getPlainTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		result := fmt.Sprintf("%02d:%02d:%02d", h, m, s)
		totalNano := ms*1000000 + us*1000 + ns
		if totalNano > 0 {
			fracStr := fmt.Sprintf(".%09d", totalNano)
			fracStr = strings.TrimRight(fracStr, "0")
			result += fracStr
		}
		return vm.NewString(result), nil
	}))

	plainTimeProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("use compare() to compare Temporal.PlainTime")
	}))

	// PlainTime constructor
	plainTimeCtor := vm.NewConstructorWithProps(0, false, "PlainTime", func(args []vm.Value) (vm.Value, error) {
		if !vmInstance.IsConstructorCall() {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainTime must be called with new")
		}
		hour, minute, second, ms, us, ns := 0, 0, 0, 0, 0, 0
		if len(args) > 0 {
			hour = int(args[0].ToFloat())
		}
		if len(args) > 1 {
			minute = int(args[1].ToFloat())
		}
		if len(args) > 2 {
			second = int(args[2].ToFloat())
		}
		if len(args) > 3 {
			ms = int(args[3].ToFloat())
		}
		if len(args) > 4 {
			us = int(args[4].ToFloat())
		}
		if len(args) > 5 {
			ns = int(args[5].ToFloat())
		}
		// Validate
		if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 {
			return vm.Undefined, vmInstance.NewRangeError("Invalid time value")
		}
		return createPlainTime(hour, minute, second, ms, us, ns), nil
	})
	plainTimeCtorProps := plainTimeCtor.AsNativeFunctionWithProps()
	plainTimeCtorProps.Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(plainTimeProto), &w, &e, &c)

	plainTimeCtorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainTime.from requires an argument")
		}
		item := args[0]
		if h, m, s, ms, us, ns, err := getPlainTimeFields(item); err == nil {
			return createPlainTime(h, m, s, ms, us, ns), nil
		}
		if item.Type() == vm.TypeString {
			return parsePlainTimeString(vmInstance, item.ToString(), createPlainTime)
		}
		return vm.Undefined, vmInstance.NewTypeError("Invalid argument for Temporal.PlainTime.from")
	}))

	plainTimeCtorProps.Properties.SetOwnNonEnumerable("compare", vm.NewNativeFunction(2, false, "compare", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("compare requires two arguments")
		}
		h1, m1, s1, ms1, us1, ns1, err := getPlainTimeFields(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		h2, m2, s2, ms2, us2, ns2, err := getPlainTimeFields(args[1])
		if err != nil {
			return vm.Undefined, err
		}
		// Compare in order: hour, minute, second, ms, us, ns
		total1 := int64(h1)*3600000000000 + int64(m1)*60000000000 + int64(s1)*1000000000 + int64(ms1)*1000000 + int64(us1)*1000 + int64(ns1)
		total2 := int64(h2)*3600000000000 + int64(m2)*60000000000 + int64(s2)*1000000000 + int64(ms2)*1000000 + int64(us2)*1000 + int64(ns2)
		if total1 < total2 {
			return vm.NumberValue(-1), nil
		}
		if total1 > total2 {
			return vm.NumberValue(1), nil
		}
		return vm.NumberValue(0), nil
	}))

	plainTimeProto.SetOwnNonEnumerable("constructor", plainTimeCtor)
	temporalObj.SetOwnNonEnumerable("PlainTime", plainTimeCtor)

	// ============================================
	// Create Temporal.PlainDateTime.prototype
	// ============================================
	plainDateTimeProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		plainDateTimeProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal.PlainDateTime"),
			&falseVal, &falseVal, &trueVal,
		)
	}

	getPlainDateTimeFields := func(val vm.Value) (year, month, day, hour, minute, second, ms, us, ns int, err error) {
		if !val.IsObject() {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainDateTime")
		}
		obj := val.AsPlainObject()
		if obj == nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainDateTime")
		}
		yVal, yOk := obj.GetOwn("[[ISOYear]]")
		moVal, moOk := obj.GetOwn("[[ISOMonth]]")
		dVal, dOk := obj.GetOwn("[[ISODay]]")
		hVal, hOk := obj.GetOwn("[[ISOHour]]")
		miVal, miOk := obj.GetOwn("[[ISOMinute]]")
		sVal, sOk := obj.GetOwn("[[ISOSecond]]")
		msVal, msOk := obj.GetOwn("[[ISOMillisecond]]")
		usVal, usOk := obj.GetOwn("[[ISOMicrosecond]]")
		nsVal, nsOk := obj.GetOwn("[[ISONanosecond]]")
		if !yOk || !moOk || !dOk || !hOk || !miOk || !sOk || !msOk || !usOk || !nsOk {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainDateTime")
		}
		return int(yVal.ToFloat()), int(moVal.ToFloat()), int(dVal.ToFloat()), int(hVal.ToFloat()), int(miVal.ToFloat()), int(sVal.ToFloat()), int(msVal.ToFloat()), int(usVal.ToFloat()), int(nsVal.ToFloat()), nil
	}

	createPlainDateTime := func(year, month, day, hour, minute, second, ms, us, ns int) vm.Value {
		dt := vm.NewObject(vm.NewValueFromPlainObject(plainDateTimeProto)).AsPlainObject()
		dt.SetOwn("[[ISOYear]]", vm.NumberValue(float64(year)))
		dt.SetOwn("[[ISOMonth]]", vm.NumberValue(float64(month)))
		dt.SetOwn("[[ISODay]]", vm.NumberValue(float64(day)))
		dt.SetOwn("[[ISOHour]]", vm.NumberValue(float64(hour)))
		dt.SetOwn("[[ISOMinute]]", vm.NumberValue(float64(minute)))
		dt.SetOwn("[[ISOSecond]]", vm.NumberValue(float64(second)))
		dt.SetOwn("[[ISOMillisecond]]", vm.NumberValue(float64(ms)))
		dt.SetOwn("[[ISOMicrosecond]]", vm.NumberValue(float64(us)))
		dt.SetOwn("[[ISONanosecond]]", vm.NumberValue(float64(ns)))
		dt.SetOwn("[[Calendar]]", vm.NewString("iso8601"))
		return vm.NewValueFromPlainObject(dt)
	}

	// PlainDateTime getters (combine date and time getters)
	plainDateTimeProto.DefineAccessorProperty("year", vm.NewNativeFunction(0, false, "get year", func(args []vm.Value) (vm.Value, error) {
		y, _, _, _, _, _, _, _, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(y)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("month", vm.NewNativeFunction(0, false, "get month", func(args []vm.Value) (vm.Value, error) {
		_, m, _, _, _, _, _, _, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(m)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("day", vm.NewNativeFunction(0, false, "get day", func(args []vm.Value) (vm.Value, error) {
		_, _, d, _, _, _, _, _, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(d)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("hour", vm.NewNativeFunction(0, false, "get hour", func(args []vm.Value) (vm.Value, error) {
		_, _, _, h, _, _, _, _, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(h)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("minute", vm.NewNativeFunction(0, false, "get minute", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, m, _, _, _, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(m)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("second", vm.NewNativeFunction(0, false, "get second", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, s, _, _, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(s)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("millisecond", vm.NewNativeFunction(0, false, "get millisecond", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, _, ms, _, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(ms)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("microsecond", vm.NewNativeFunction(0, false, "get microsecond", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, _, _, us, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(us)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("nanosecond", vm.NewNativeFunction(0, false, "get nanosecond", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, _, _, _, ns, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(ns)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("calendarId", vm.NewNativeFunction(0, false, "get calendarId", func(args []vm.Value) (vm.Value, error) {
		return vm.NewString("iso8601"), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("dayOfWeek", vm.NewNativeFunction(0, false, "get dayOfWeek", func(args []vm.Value) (vm.Value, error) {
		y, m, d, _, _, _, _, _, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		dow := int(t.Weekday())
		if dow == 0 {
			dow = 7
		}
		return vm.NumberValue(float64(dow)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.DefineAccessorProperty("dayOfYear", vm.NewNativeFunction(0, false, "get dayOfYear", func(args []vm.Value) (vm.Value, error) {
		y, m, d, _, _, _, _, _, _, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		return vm.NumberValue(float64(t.YearDay())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainDateTimeProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		y, mo, d, h, mi, s, ms, us, ns, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		result := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d", y, mo, d, h, mi, s)
		totalNano := ms*1000000 + us*1000 + ns
		if totalNano > 0 {
			fracStr := fmt.Sprintf(".%09d", totalNano)
			fracStr = strings.TrimRight(fracStr, "0")
			result += fracStr
		}
		return vm.NewString(result), nil
	}))

	plainDateTimeProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		y, mo, d, h, mi, s, ms, us, ns, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		result := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d", y, mo, d, h, mi, s)
		totalNano := ms*1000000 + us*1000 + ns
		if totalNano > 0 {
			fracStr := fmt.Sprintf(".%09d", totalNano)
			fracStr = strings.TrimRight(fracStr, "0")
			result += fracStr
		}
		return vm.NewString(result), nil
	}))

	plainDateTimeProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("use compare() to compare Temporal.PlainDateTime")
	}))

	// PlainDateTime constructor
	plainDateTimeCtor := vm.NewConstructorWithProps(3, false, "PlainDateTime", func(args []vm.Value) (vm.Value, error) {
		if !vmInstance.IsConstructorCall() {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainDateTime must be called with new")
		}
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainDateTime requires at least year, month, day")
		}
		year := int(args[0].ToFloat())
		month := int(args[1].ToFloat())
		day := int(args[2].ToFloat())
		hour, minute, second, ms, us, ns := 0, 0, 0, 0, 0, 0
		if len(args) > 3 {
			hour = int(args[3].ToFloat())
		}
		if len(args) > 4 {
			minute = int(args[4].ToFloat())
		}
		if len(args) > 5 {
			second = int(args[5].ToFloat())
		}
		if len(args) > 6 {
			ms = int(args[6].ToFloat())
		}
		if len(args) > 7 {
			us = int(args[7].ToFloat())
		}
		if len(args) > 8 {
			ns = int(args[8].ToFloat())
		}
		return createPlainDateTime(year, month, day, hour, minute, second, ms, us, ns), nil
	})
	plainDateTimeCtorProps := plainDateTimeCtor.AsNativeFunctionWithProps()
	plainDateTimeCtorProps.Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(plainDateTimeProto), &w, &e, &c)

	plainDateTimeCtorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainDateTime.from requires an argument")
		}
		item := args[0]
		if y, mo, d, h, mi, s, ms, us, ns, err := getPlainDateTimeFields(item); err == nil {
			return createPlainDateTime(y, mo, d, h, mi, s, ms, us, ns), nil
		}
		if item.Type() == vm.TypeString {
			return parsePlainDateTimeString(vmInstance, item.ToString(), createPlainDateTime)
		}
		return vm.Undefined, vmInstance.NewTypeError("Invalid argument for Temporal.PlainDateTime.from")
	}))

	plainDateTimeCtorProps.Properties.SetOwnNonEnumerable("compare", vm.NewNativeFunction(2, false, "compare", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("compare requires two arguments")
		}
		y1, mo1, d1, h1, mi1, s1, ms1, us1, ns1, err := getPlainDateTimeFields(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		y2, mo2, d2, h2, mi2, s2, ms2, us2, ns2, err := getPlainDateTimeFields(args[1])
		if err != nil {
			return vm.Undefined, err
		}
		// Compare fields in order
		if y1 != y2 {
			if y1 < y2 {
				return vm.NumberValue(-1), nil
			}
			return vm.NumberValue(1), nil
		}
		if mo1 != mo2 {
			if mo1 < mo2 {
				return vm.NumberValue(-1), nil
			}
			return vm.NumberValue(1), nil
		}
		if d1 != d2 {
			if d1 < d2 {
				return vm.NumberValue(-1), nil
			}
			return vm.NumberValue(1), nil
		}
		// Compare time part
		total1 := int64(h1)*3600000000000 + int64(mi1)*60000000000 + int64(s1)*1000000000 + int64(ms1)*1000000 + int64(us1)*1000 + int64(ns1)
		total2 := int64(h2)*3600000000000 + int64(mi2)*60000000000 + int64(s2)*1000000000 + int64(ms2)*1000000 + int64(us2)*1000 + int64(ns2)
		if total1 < total2 {
			return vm.NumberValue(-1), nil
		}
		if total1 > total2 {
			return vm.NumberValue(1), nil
		}
		return vm.NumberValue(0), nil
	}))

	plainDateTimeProto.SetOwnNonEnumerable("constructor", plainDateTimeCtor)
	temporalObj.SetOwnNonEnumerable("PlainDateTime", plainDateTimeCtor)

	// ============================================
	// Create Temporal.Now object
	// ============================================
	temporalNowObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		temporalNowObj.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal.Now"),
			&falseVal, &falseVal, &trueVal,
		)
	}

	temporalNowObj.SetOwnNonEnumerable("timeZoneId", vm.NewNativeFunction(0, false, "timeZoneId", func(args []vm.Value) (vm.Value, error) {
		zone, _ := time.Now().Zone()
		loc := time.Local
		if loc != nil && loc.String() != "Local" {
			return vm.NewString(loc.String()), nil
		}
		if zone != "" {
			return vm.NewString(zone), nil
		}
		return vm.NewString("UTC"), nil
	}))

	temporalNowObj.SetOwnNonEnumerable("instant", vm.NewNativeFunction(0, false, "instant", func(args []vm.Value) (vm.Value, error) {
		now := time.Now()
		nanos := new(big.Int).SetInt64(now.UnixNano())
		return createInstant(nanos), nil
	}))

	temporalNowObj.SetOwnNonEnumerable("plainDateISO", vm.NewNativeFunction(0, false, "plainDateISO", func(args []vm.Value) (vm.Value, error) {
		now := time.Now()
		return createPlainDate(now.Year(), int(now.Month()), now.Day()), nil
	}))

	temporalNowObj.SetOwnNonEnumerable("plainTimeISO", vm.NewNativeFunction(0, false, "plainTimeISO", func(args []vm.Value) (vm.Value, error) {
		now := time.Now()
		ns := now.Nanosecond()
		ms := ns / 1000000
		us := (ns % 1000000) / 1000
		nsRem := ns % 1000
		return createPlainTime(now.Hour(), now.Minute(), now.Second(), ms, us, nsRem), nil
	}))

	temporalNowObj.SetOwnNonEnumerable("plainDateTimeISO", vm.NewNativeFunction(0, false, "plainDateTimeISO", func(args []vm.Value) (vm.Value, error) {
		now := time.Now()
		ns := now.Nanosecond()
		ms := ns / 1000000
		us := (ns % 1000000) / 1000
		nsRem := ns % 1000
		return createPlainDateTime(now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second(), ms, us, nsRem), nil
	}))

	temporalNowObj.SetOwnNonEnumerable("zonedDateTimeISO", vm.NewNativeFunction(0, false, "zonedDateTimeISO", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("Temporal.ZonedDateTime not yet implemented")
	}))

	temporalObj.SetOwnNonEnumerable("Now", vm.NewValueFromPlainObject(temporalNowObj))

	return ctx.DefineGlobal("Temporal", vm.NewValueFromPlainObject(temporalObj))
}

// parseInstantString parses an ISO 8601 instant string
func parseInstantString(vmInstance *vm.VM, str string, createInstant func(*big.Int) vm.Value) (vm.Value, error) {
	// Temporal.Instant requires a UTC offset or Z
	// Format: YYYY-MM-DDTHH:mm:ss.sssssssss+HH:MM or ...Z

	// Try parsing with Z suffix
	layouts := []string{
		"2006-01-02T15:04:05.999999999Z",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05.999999999+07:00",
		"2006-01-02T15:04:05+07:00",
	}

	var t time.Time
	var err error
	parsed := false

	for _, layout := range layouts {
		t, err = time.Parse(layout, str)
		if err == nil {
			parsed = true
			break
		}
	}

	if !parsed {
		// Try more flexible parsing for various offset formats
		// Handle cases like "2024-01-15T10:30:00+00:00"
		if idx := strings.LastIndex(str, "+"); idx > 0 {
			base := str[:idx]
			offset := str[idx:]
			// Try to parse the offset
			if len(offset) == 6 && offset[3] == ':' { // +HH:MM format
				hours, _ := strconv.Atoi(offset[1:3])
				mins, _ := strconv.Atoi(offset[4:6])
				totalMins := hours*60 + mins
				loc := time.FixedZone("", totalMins*60)
				for _, baseLayout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
					if t, err = time.ParseInLocation(baseLayout, base, loc); err == nil {
						parsed = true
						break
					}
				}
			}
		} else if idx := strings.LastIndex(str, "-"); idx > 10 { // After date part
			base := str[:idx]
			offset := str[idx:]
			if len(offset) == 6 && offset[3] == ':' { // -HH:MM format
				hours, _ := strconv.Atoi(offset[1:3])
				mins, _ := strconv.Atoi(offset[4:6])
				totalMins := -(hours*60 + mins)
				loc := time.FixedZone("", totalMins*60)
				for _, baseLayout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
					if t, err = time.ParseInLocation(baseLayout, base, loc); err == nil {
						parsed = true
						break
					}
				}
			}
		}
	}

	if !parsed {
		return vm.Undefined, vmInstance.NewRangeError("Invalid Temporal.Instant string: " + str)
	}

	// Convert to UTC nanoseconds
	nanos := new(big.Int).SetInt64(t.UTC().UnixNano())
	return createInstant(nanos), nil
}

// isLeapYear returns true if the year is a leap year
func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// parsePlainDateString parses an ISO 8601 date string (YYYY-MM-DD)
func parsePlainDateString(vmInstance *vm.VM, str string, createPlainDate func(int, int, int) vm.Value) (vm.Value, error) {
	// Try standard ISO format first
	t, err := time.Parse("2006-01-02", str)
	if err == nil {
		return createPlainDate(t.Year(), int(t.Month()), t.Day()), nil
	}

	// Try with time component (take just the date part)
	if idx := strings.Index(str, "T"); idx > 0 {
		dateStr := str[:idx]
		t, err = time.Parse("2006-01-02", dateStr)
		if err == nil {
			return createPlainDate(t.Year(), int(t.Month()), t.Day()), nil
		}
	}

	// Try parsing extended year format (e.g., +006789-01-15)
	if len(str) >= 10 && (str[0] == '+' || str[0] == '-') {
		// Extended year format: [+-]YYYYYY-MM-DD
		// Find the second hyphen (after year)
		hyphenCount := 0
		yearEnd := 0
		for i, c := range str[1:] {
			if c == '-' {
				hyphenCount++
				if hyphenCount == 1 && i >= 4 {
					yearEnd = i + 1
					break
				}
			}
		}
		if yearEnd > 0 && len(str) >= yearEnd+6 {
			yearStr := str[:yearEnd]
			monthStr := str[yearEnd+1 : yearEnd+3]
			dayStr := str[yearEnd+4 : yearEnd+6]

			year, err1 := strconv.Atoi(yearStr)
			month, err2 := strconv.Atoi(monthStr)
			day, err3 := strconv.Atoi(dayStr)

			if err1 == nil && err2 == nil && err3 == nil && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
				return createPlainDate(year, month, day), nil
			}
		}
	}

	return vm.Undefined, vmInstance.NewRangeError("Invalid Temporal.PlainDate string: " + str)
}

// parsePlainTimeString parses an ISO 8601 time string (HH:MM:SS.sssssssss)
func parsePlainTimeString(vmInstance *vm.VM, str string, createPlainTime func(int, int, int, int, int, int) vm.Value) (vm.Value, error) {
	// Remove any trailing timezone info for plain time
	if idx := strings.Index(str, "Z"); idx > 0 {
		str = str[:idx]
	}
	if idx := strings.Index(str, "+"); idx > 0 {
		str = str[:idx]
	}
	if idx := strings.LastIndex(str, "-"); idx > 2 {
		// Check if this looks like a timezone offset (not a date separator)
		remaining := str[idx:]
		if len(remaining) >= 3 && remaining[1] >= '0' && remaining[1] <= '2' {
			str = str[:idx]
		}
	}

	// If it has a T separator, extract the time part
	if idx := strings.Index(str, "T"); idx >= 0 {
		str = str[idx+1:]
	}

	// Parse components
	hour, minute, second := 0, 0, 0
	millisecond, microsecond, nanosecond := 0, 0, 0

	parts := strings.Split(str, ":")
	if len(parts) < 2 {
		return vm.Undefined, vmInstance.NewRangeError("Invalid Temporal.PlainTime string: " + str)
	}

	var err error
	hour, err = strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return vm.Undefined, vmInstance.NewRangeError("Invalid hour in Temporal.PlainTime string")
	}

	minute, err = strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return vm.Undefined, vmInstance.NewRangeError("Invalid minute in Temporal.PlainTime string")
	}

	if len(parts) >= 3 {
		secPart := parts[2]
		// Check for fractional seconds
		if dotIdx := strings.Index(secPart, "."); dotIdx >= 0 {
			second, err = strconv.Atoi(secPart[:dotIdx])
			if err != nil || second < 0 || second > 59 {
				return vm.Undefined, vmInstance.NewRangeError("Invalid second in Temporal.PlainTime string")
			}

			frac := secPart[dotIdx+1:]
			// Pad or truncate to 9 digits (nanoseconds)
			for len(frac) < 9 {
				frac += "0"
			}
			if len(frac) > 9 {
				frac = frac[:9]
			}

			nanos, _ := strconv.Atoi(frac)
			millisecond = nanos / 1000000
			microsecond = (nanos / 1000) % 1000
			nanosecond = nanos % 1000
		} else {
			second, err = strconv.Atoi(secPart)
			if err != nil || second < 0 || second > 59 {
				return vm.Undefined, vmInstance.NewRangeError("Invalid second in Temporal.PlainTime string")
			}
		}
	}

	return createPlainTime(hour, minute, second, millisecond, microsecond, nanosecond), nil
}

// parsePlainDateTimeString parses an ISO 8601 date-time string (YYYY-MM-DDTHH:MM:SS.sssssssss)
func parsePlainDateTimeString(vmInstance *vm.VM, str string, createPlainDateTime func(int, int, int, int, int, int, int, int, int) vm.Value) (vm.Value, error) {
	// Remove any trailing timezone info for plain datetime
	cleanStr := str
	if idx := strings.Index(cleanStr, "Z"); idx > 0 {
		cleanStr = cleanStr[:idx]
	}
	if idx := strings.Index(cleanStr, "["); idx > 0 {
		cleanStr = cleanStr[:idx]
	}

	// Check for timezone offset after time (but not date separator)
	// Look for +/- after the time portion
	if tIdx := strings.Index(cleanStr, "T"); tIdx > 0 {
		timePart := cleanStr[tIdx+1:]
		if idx := strings.Index(timePart, "+"); idx > 0 {
			cleanStr = cleanStr[:tIdx+1+idx]
		} else if idx := strings.LastIndex(timePart, "-"); idx > 2 {
			// Make sure it's a timezone offset, not part of time
			remaining := timePart[idx:]
			if len(remaining) >= 3 && remaining[1] >= '0' && remaining[1] <= '2' {
				cleanStr = cleanStr[:tIdx+1+idx]
			}
		}
	}

	// Split into date and time parts
	var datePart, timePart string
	if idx := strings.Index(cleanStr, "T"); idx > 0 {
		datePart = cleanStr[:idx]
		timePart = cleanStr[idx+1:]
	} else if idx := strings.Index(cleanStr, " "); idx > 0 {
		datePart = cleanStr[:idx]
		timePart = cleanStr[idx+1:]
	} else {
		// Just a date
		datePart = cleanStr
		timePart = "00:00:00"
	}

	// Parse date
	var year, month, day int
	t, err := time.Parse("2006-01-02", datePart)
	if err == nil {
		year, month, day = t.Year(), int(t.Month()), t.Day()
	} else {
		// Try extended year format
		return vm.Undefined, vmInstance.NewRangeError("Invalid Temporal.PlainDateTime date: " + datePart)
	}

	// Parse time components
	hour, minute, second := 0, 0, 0
	millisecond, microsecond, nanosecond := 0, 0, 0

	if timePart != "" {
		parts := strings.Split(timePart, ":")
		if len(parts) >= 1 {
			hour, err = strconv.Atoi(parts[0])
			if err != nil || hour < 0 || hour > 23 {
				return vm.Undefined, vmInstance.NewRangeError("Invalid hour in Temporal.PlainDateTime string")
			}
		}
		if len(parts) >= 2 {
			minute, err = strconv.Atoi(parts[1])
			if err != nil || minute < 0 || minute > 59 {
				return vm.Undefined, vmInstance.NewRangeError("Invalid minute in Temporal.PlainDateTime string")
			}
		}
		if len(parts) >= 3 {
			secPart := parts[2]
			if dotIdx := strings.Index(secPart, "."); dotIdx >= 0 {
				second, err = strconv.Atoi(secPart[:dotIdx])
				if err != nil || second < 0 || second > 59 {
					return vm.Undefined, vmInstance.NewRangeError("Invalid second in Temporal.PlainDateTime string")
				}

				frac := secPart[dotIdx+1:]
				for len(frac) < 9 {
					frac += "0"
				}
				if len(frac) > 9 {
					frac = frac[:9]
				}

				nanos, _ := strconv.Atoi(frac)
				millisecond = nanos / 1000000
				microsecond = (nanos / 1000) % 1000
				nanosecond = nanos % 1000
			} else {
				second, err = strconv.Atoi(secPart)
				if err != nil || second < 0 || second > 59 {
					return vm.Undefined, vmInstance.NewRangeError("Invalid second in Temporal.PlainDateTime string")
				}
			}
		}
	}

	return createPlainDateTime(year, month, day, hour, minute, second, millisecond, microsecond, nanosecond), nil
}

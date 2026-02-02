package builtins

import (
	"fmt"
	"math"
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

	// Helper to get duration fields from a value (Duration or duration-like object)
	getDurationFromArg := func(arg vm.Value) (years, months, weeks, days, hours, minutes, seconds, ms, us, ns int, err error) {
		if !arg.IsObject() {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Invalid duration")
		}
		obj := arg.AsPlainObject()
		if obj == nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Invalid duration")
		}
		// Try internal slots first (for Temporal.Duration objects)
		if yVal, ok := obj.GetOwn("[[Years]]"); ok {
			yearsVal, _ := obj.GetOwn("[[Years]]")
			monthsVal, _ := obj.GetOwn("[[Months]]")
			weeksVal, _ := obj.GetOwn("[[Weeks]]")
			daysVal, _ := obj.GetOwn("[[Days]]")
			hoursVal, _ := obj.GetOwn("[[Hours]]")
			minutesVal, _ := obj.GetOwn("[[Minutes]]")
			secondsVal, _ := obj.GetOwn("[[Seconds]]")
			msVal, _ := obj.GetOwn("[[Milliseconds]]")
			usVal, _ := obj.GetOwn("[[Microseconds]]")
			nsVal, _ := obj.GetOwn("[[Nanoseconds]]")
			_ = yVal
			return int(yearsVal.ToFloat()), int(monthsVal.ToFloat()), int(weeksVal.ToFloat()), int(daysVal.ToFloat()),
				int(hoursVal.ToFloat()), int(minutesVal.ToFloat()), int(secondsVal.ToFloat()),
				int(msVal.ToFloat()), int(usVal.ToFloat()), int(nsVal.ToFloat()), nil
		}

		// For duration-like objects, check for singular property names (which are invalid)
		singularProps := []string{"year", "month", "week", "day", "hour", "minute", "second", "millisecond", "microsecond", "nanosecond"}
		for _, prop := range singularProps {
			if val, ok := obj.Get(prop); ok && val.Type() != vm.TypeUndefined {
				return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Invalid duration: use plural property names")
			}
		}

		// Try property access for duration-like objects
		yearsVal, hasYears := obj.Get("years")
		monthsVal, hasMonths := obj.Get("months")
		weeksVal, hasWeeks := obj.Get("weeks")
		daysVal, hasDays := obj.Get("days")
		hoursVal, hasHours := obj.Get("hours")
		minutesVal, hasMinutes := obj.Get("minutes")
		secondsVal, hasSeconds := obj.Get("seconds")
		msVal, hasMs := obj.Get("milliseconds")
		usVal, hasUs := obj.Get("microseconds")
		nsVal, hasNs := obj.Get("nanoseconds")

		// Check that at least one valid property is present and not undefined
		hasValidProp := false
		if hasYears && yearsVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}
		if hasMonths && monthsVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}
		if hasWeeks && weeksVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}
		if hasDays && daysVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}
		if hasHours && hoursVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}
		if hasMinutes && minutesVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}
		if hasSeconds && secondsVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}
		if hasMs && msVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}
		if hasUs && usVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}
		if hasNs && nsVal.Type() != vm.TypeUndefined {
			hasValidProp = true
		}

		if !hasValidProp {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Invalid duration: no valid properties")
		}

		// Convert each value using ToIntegerIfIntegral (validates Symbol/BigInt)
		yearsInt, err := toIntegerIfIntegral(vmInstance, yearsVal, "years")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}
		monthsInt, err := toIntegerIfIntegral(vmInstance, monthsVal, "months")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}
		weeksInt, err := toIntegerIfIntegral(vmInstance, weeksVal, "weeks")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}
		daysInt, err := toIntegerIfIntegral(vmInstance, daysVal, "days")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}
		hoursInt, err := toIntegerIfIntegral(vmInstance, hoursVal, "hours")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}
		minutesInt, err := toIntegerIfIntegral(vmInstance, minutesVal, "minutes")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}
		secondsInt, err := toIntegerIfIntegral(vmInstance, secondsVal, "seconds")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}
		msInt, err := toIntegerIfIntegral(vmInstance, msVal, "milliseconds")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}
		usInt, err := toIntegerIfIntegral(vmInstance, usVal, "microseconds")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}
		nsInt, err := toIntegerIfIntegral(vmInstance, nsVal, "nanoseconds")
		if err != nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
		}

		return yearsInt, monthsInt, weeksInt, daysInt, hoursInt, minutesInt, secondsInt, msInt, usInt, nsInt, nil
	}

	// Temporal.Instant.prototype.add(duration [, options])
	instantProto.SetOwnNonEnumerable("add", vm.NewNativeFunction(1, false, "add", func(args []vm.Value) (vm.Value, error) {
		thisNanos, err := getInstantNanos(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("add requires a duration argument")
		}
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
		}
		years, months, weeks, days, hours, minutes, seconds, ms, us, ns, err := getDurationFromArg(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// Instant.add() doesn't allow years, months, weeks, or days
		if years != 0 || months != 0 || weeks != 0 || days != 0 {
			return vm.Undefined, vmInstance.NewRangeError("Instant.add() does not support years, months, weeks, or days")
		}
		// Calculate nanoseconds to add
		totalNanos := int64(hours)*3600*1e9 + int64(minutes)*60*1e9 + int64(seconds)*1e9 +
			int64(ms)*1e6 + int64(us)*1e3 + int64(ns)
		result := new(big.Int).Add(thisNanos, big.NewInt(totalNanos))
		return createInstant(result), nil
	}))

	// Temporal.Instant.prototype.subtract(duration [, options])
	instantProto.SetOwnNonEnumerable("subtract", vm.NewNativeFunction(1, false, "subtract", func(args []vm.Value) (vm.Value, error) {
		thisNanos, err := getInstantNanos(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("subtract requires a duration argument")
		}
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
		}
		years, months, weeks, days, hours, minutes, seconds, ms, us, ns, err := getDurationFromArg(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// Instant.subtract() doesn't allow years, months, weeks, or days
		if years != 0 || months != 0 || weeks != 0 || days != 0 {
			return vm.Undefined, vmInstance.NewRangeError("Instant.subtract() does not support years, months, weeks, or days")
		}
		// Calculate nanoseconds to subtract
		totalNanos := int64(hours)*3600*1e9 + int64(minutes)*60*1e9 + int64(seconds)*1e9 +
			int64(ms)*1e6 + int64(us)*1e3 + int64(ns)
		result := new(big.Int).Sub(thisNanos, big.NewInt(totalNanos))
		return createInstant(result), nil
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

	// Temporal.PlainDate.prototype.add(duration [, options])
	plainDateProto.SetOwnNonEnumerable("add", vm.NewNativeFunction(1, false, "add", func(args []vm.Value) (vm.Value, error) {
		y, m, d, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("add requires a duration argument")
		}
		// Read overflow option FIRST (before algorithmic validation per spec)
		var options vm.Value = vm.Undefined
		if len(args) > 1 {
			options = args[1]
		}
		if err := validateTemporalOptions(vmInstance, options); err != nil {
			return vm.Undefined, err
		}
		overflow, err := getOverflowOption(vmInstance, options)
		if err != nil {
			return vm.Undefined, err
		}
		years, months, weeks, days, _, _, _, _, _, _, err := getDurationFromArg(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// Add duration to date with overflow handling
		newY, newM, newD, err := addDateWithOverflow(vmInstance, y, m, d, years, months, weeks*7+days, overflow)
		if err != nil {
			return vm.Undefined, err
		}
		return createPlainDate(newY, newM, newD), nil
	}))

	// Temporal.PlainDate.prototype.subtract(duration [, options])
	plainDateProto.SetOwnNonEnumerable("subtract", vm.NewNativeFunction(1, false, "subtract", func(args []vm.Value) (vm.Value, error) {
		y, m, d, err := getPlainDateFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("subtract requires a duration argument")
		}
		// Read overflow option FIRST (before algorithmic validation per spec)
		var options vm.Value = vm.Undefined
		if len(args) > 1 {
			options = args[1]
		}
		if err := validateTemporalOptions(vmInstance, options); err != nil {
			return vm.Undefined, err
		}
		overflow, err := getOverflowOption(vmInstance, options)
		if err != nil {
			return vm.Undefined, err
		}
		years, months, weeks, days, _, _, _, _, _, _, err := getDurationFromArg(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// Subtract duration from date with overflow handling
		newY, newM, newD, err := addDateWithOverflow(vmInstance, y, m, d, -years, -months, -(weeks*7 + days), overflow)
		if err != nil {
			return vm.Undefined, err
		}
		return createPlainDate(newY, newM, newD), nil
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
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
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
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
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

	// Temporal.PlainDateTime.prototype.add(duration [, options])
	plainDateTimeProto.SetOwnNonEnumerable("add", vm.NewNativeFunction(1, false, "add", func(args []vm.Value) (vm.Value, error) {
		y, mo, d, h, mi, s, ms, us, ns, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("add requires a duration argument")
		}
		// Read overflow option FIRST (before algorithmic validation per spec)
		var options vm.Value = vm.Undefined
		if len(args) > 1 {
			options = args[1]
		}
		if err := validateTemporalOptions(vmInstance, options); err != nil {
			return vm.Undefined, err
		}
		overflow, err := getOverflowOption(vmInstance, options)
		if err != nil {
			return vm.Undefined, err
		}
		years, months, weeks, days, hours, minutes, seconds, dms, dus, dns, err := getDurationFromArg(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// Add date parts with overflow handling
		newY, newMo, newD, err := addDateWithOverflow(vmInstance, y, mo, d, years, months, weeks*7+days, overflow)
		if err != nil {
			return vm.Undefined, err
		}
		// Convert to time.Time and add time parts
		totalNanos := ms*1000000 + us*1000 + ns
		t := time.Date(newY, time.Month(newMo), newD, h, mi, s, totalNanos, time.UTC)
		// Add time parts
		t = t.Add(time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute +
			time.Duration(seconds)*time.Second + time.Duration(dms)*time.Millisecond +
			time.Duration(dus)*time.Microsecond + time.Duration(dns)*time.Nanosecond)
		newNs := t.Nanosecond()
		return createPlainDateTime(t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second(),
			newNs/1000000, (newNs/1000)%1000, newNs%1000), nil
	}))

	// Temporal.PlainDateTime.prototype.subtract(duration [, options])
	plainDateTimeProto.SetOwnNonEnumerable("subtract", vm.NewNativeFunction(1, false, "subtract", func(args []vm.Value) (vm.Value, error) {
		y, mo, d, h, mi, s, ms, us, ns, err := getPlainDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("subtract requires a duration argument")
		}
		// Read overflow option FIRST (before algorithmic validation per spec)
		var options vm.Value = vm.Undefined
		if len(args) > 1 {
			options = args[1]
		}
		if err := validateTemporalOptions(vmInstance, options); err != nil {
			return vm.Undefined, err
		}
		overflow, err := getOverflowOption(vmInstance, options)
		if err != nil {
			return vm.Undefined, err
		}
		years, months, weeks, days, hours, minutes, seconds, dms, dus, dns, err := getDurationFromArg(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// Subtract date parts with overflow handling
		newY, newMo, newD, err := addDateWithOverflow(vmInstance, y, mo, d, -years, -months, -(weeks*7 + days), overflow)
		if err != nil {
			return vm.Undefined, err
		}
		// Convert to time.Time and subtract time parts
		totalNanos := ms*1000000 + us*1000 + ns
		t := time.Date(newY, time.Month(newMo), newD, h, mi, s, totalNanos, time.UTC)
		// Subtract time parts
		t = t.Add(-(time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute +
			time.Duration(seconds)*time.Second + time.Duration(dms)*time.Millisecond +
			time.Duration(dus)*time.Microsecond + time.Duration(dns)*time.Nanosecond))
		newNs := t.Nanosecond()
		return createPlainDateTime(t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second(),
			newNs/1000000, (newNs/1000)%1000, newNs%1000), nil
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
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
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
	// Create Temporal.PlainYearMonth.prototype
	// ============================================
	plainYearMonthProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		plainYearMonthProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal.PlainYearMonth"),
			&falseVal, &falseVal, &trueVal,
		)
	}

	getPlainYearMonthFields := func(val vm.Value) (year, month int, err error) {
		if !val.IsObject() {
			return 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainYearMonth")
		}
		obj := val.AsPlainObject()
		if obj == nil {
			return 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainYearMonth")
		}
		yVal, yOk := obj.GetOwn("[[ISOYear]]")
		mVal, mOk := obj.GetOwn("[[ISOMonth]]")
		if !yOk || !mOk {
			return 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainYearMonth")
		}
		return int(yVal.ToFloat()), int(mVal.ToFloat()), nil
	}

	createPlainYearMonth := func(year, month int) vm.Value {
		ym := vm.NewObject(vm.NewValueFromPlainObject(plainYearMonthProto)).AsPlainObject()
		ym.SetOwn("[[ISOYear]]", vm.NumberValue(float64(year)))
		ym.SetOwn("[[ISOMonth]]", vm.NumberValue(float64(month)))
		ym.SetOwn("[[ISODay]]", vm.NumberValue(1)) // Reference day
		ym.SetOwn("[[Calendar]]", vm.NewString("iso8601"))
		return vm.NewValueFromPlainObject(ym)
	}

	plainYearMonthProto.DefineAccessorProperty("year", vm.NewNativeFunction(0, false, "get year", func(args []vm.Value) (vm.Value, error) {
		y, _, err := getPlainYearMonthFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(y)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainYearMonthProto.DefineAccessorProperty("month", vm.NewNativeFunction(0, false, "get month", func(args []vm.Value) (vm.Value, error) {
		_, m, err := getPlainYearMonthFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(m)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainYearMonthProto.DefineAccessorProperty("calendarId", vm.NewNativeFunction(0, false, "get calendarId", func(args []vm.Value) (vm.Value, error) {
		return vm.NewString("iso8601"), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainYearMonthProto.DefineAccessorProperty("daysInMonth", vm.NewNativeFunction(0, false, "get daysInMonth", func(args []vm.Value) (vm.Value, error) {
		y, m, err := getPlainYearMonthFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		nextMonth := time.Date(y, time.Month(m)+1, 1, 0, 0, 0, 0, time.UTC)
		lastDay := nextMonth.AddDate(0, 0, -1)
		return vm.NumberValue(float64(lastDay.Day())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainYearMonthProto.DefineAccessorProperty("daysInYear", vm.NewNativeFunction(0, false, "get daysInYear", func(args []vm.Value) (vm.Value, error) {
		y, _, err := getPlainYearMonthFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if isLeapYear(y) {
			return vm.NumberValue(366), nil
		}
		return vm.NumberValue(365), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainYearMonthProto.DefineAccessorProperty("monthsInYear", vm.NewNativeFunction(0, false, "get monthsInYear", func(args []vm.Value) (vm.Value, error) {
		return vm.NumberValue(12), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainYearMonthProto.DefineAccessorProperty("inLeapYear", vm.NewNativeFunction(0, false, "get inLeapYear", func(args []vm.Value) (vm.Value, error) {
		y, _, err := getPlainYearMonthFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(isLeapYear(y)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainYearMonthProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		y, m, err := getPlainYearMonthFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(fmt.Sprintf("%04d-%02d", y, m)), nil
	}))

	plainYearMonthProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		y, m, err := getPlainYearMonthFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(fmt.Sprintf("%04d-%02d", y, m)), nil
	}))

	plainYearMonthProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("use compare() to compare Temporal.PlainYearMonth")
	}))

	plainYearMonthCtor := vm.NewConstructorWithProps(2, false, "PlainYearMonth", func(args []vm.Value) (vm.Value, error) {
		if !vmInstance.IsConstructorCall() {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainYearMonth must be called with new")
		}
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainYearMonth requires year and month")
		}
		// Check for Infinity/NaN
		if err := checkTemporalArgs(vmInstance, args, []string{"year", "month", "calendar", "referenceISODay"}); err != nil {
			return vm.Undefined, err
		}
		// Validate calendar if provided (3rd argument)
		if len(args) > 2 {
			if err := validateCalendarID(vmInstance, args[2]); err != nil {
				return vm.Undefined, err
			}
		}
		year := int(args[0].ToFloat())
		month := int(args[1].ToFloat())
		if month < 1 || month > 12 {
			return vm.Undefined, vmInstance.NewRangeError("Invalid month")
		}
		return createPlainYearMonth(year, month), nil
	})
	plainYearMonthCtorProps := plainYearMonthCtor.AsNativeFunctionWithProps()
	plainYearMonthCtorProps.Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(plainYearMonthProto), &w, &e, &c)

	plainYearMonthCtorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainYearMonth.from requires an argument")
		}
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
		}
		item := args[0]
		if y, m, err := getPlainYearMonthFields(item); err == nil {
			return createPlainYearMonth(y, m), nil
		}
		if item.Type() == vm.TypeString {
			str := item.ToString()
			t, err := time.Parse("2006-01", str)
			if err == nil {
				return createPlainYearMonth(t.Year(), int(t.Month())), nil
			}
			// Try with day component
			t, err = time.Parse("2006-01-02", str)
			if err == nil {
				return createPlainYearMonth(t.Year(), int(t.Month())), nil
			}
			return vm.Undefined, vmInstance.NewRangeError("Invalid Temporal.PlainYearMonth string")
		}
		return vm.Undefined, vmInstance.NewTypeError("Invalid argument for Temporal.PlainYearMonth.from")
	}))

	plainYearMonthCtorProps.Properties.SetOwnNonEnumerable("compare", vm.NewNativeFunction(2, false, "compare", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("compare requires two arguments")
		}
		y1, m1, err := getPlainYearMonthFields(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		y2, m2, err := getPlainYearMonthFields(args[1])
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
		return vm.NumberValue(0), nil
	}))

	plainYearMonthProto.SetOwnNonEnumerable("constructor", plainYearMonthCtor)
	temporalObj.SetOwnNonEnumerable("PlainYearMonth", plainYearMonthCtor)

	// ============================================
	// Create Temporal.PlainMonthDay.prototype
	// ============================================
	plainMonthDayProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		plainMonthDayProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal.PlainMonthDay"),
			&falseVal, &falseVal, &trueVal,
		)
	}

	getPlainMonthDayFields := func(val vm.Value) (month, day int, err error) {
		if !val.IsObject() {
			return 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainMonthDay")
		}
		obj := val.AsPlainObject()
		if obj == nil {
			return 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainMonthDay")
		}
		mVal, mOk := obj.GetOwn("[[ISOMonth]]")
		dVal, dOk := obj.GetOwn("[[ISODay]]")
		if !mOk || !dOk {
			return 0, 0, vmInstance.NewTypeError("Value is not a Temporal.PlainMonthDay")
		}
		return int(mVal.ToFloat()), int(dVal.ToFloat()), nil
	}

	createPlainMonthDay := func(month, day int) vm.Value {
		md := vm.NewObject(vm.NewValueFromPlainObject(plainMonthDayProto)).AsPlainObject()
		md.SetOwn("[[ISOYear]]", vm.NumberValue(1972)) // Reference year (leap year)
		md.SetOwn("[[ISOMonth]]", vm.NumberValue(float64(month)))
		md.SetOwn("[[ISODay]]", vm.NumberValue(float64(day)))
		md.SetOwn("[[Calendar]]", vm.NewString("iso8601"))
		return vm.NewValueFromPlainObject(md)
	}

	plainMonthDayProto.DefineAccessorProperty("monthCode", vm.NewNativeFunction(0, false, "get monthCode", func(args []vm.Value) (vm.Value, error) {
		m, _, err := getPlainMonthDayFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(fmt.Sprintf("M%02d", m)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainMonthDayProto.DefineAccessorProperty("day", vm.NewNativeFunction(0, false, "get day", func(args []vm.Value) (vm.Value, error) {
		_, d, err := getPlainMonthDayFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(d)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainMonthDayProto.DefineAccessorProperty("calendarId", vm.NewNativeFunction(0, false, "get calendarId", func(args []vm.Value) (vm.Value, error) {
		return vm.NewString("iso8601"), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	plainMonthDayProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		m, d, err := getPlainMonthDayFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(fmt.Sprintf("%02d-%02d", m, d)), nil
	}))

	plainMonthDayProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		m, d, err := getPlainMonthDayFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(fmt.Sprintf("%02d-%02d", m, d)), nil
	}))

	plainMonthDayProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("use equals() to compare Temporal.PlainMonthDay")
	}))

	plainMonthDayCtor := vm.NewConstructorWithProps(2, false, "PlainMonthDay", func(args []vm.Value) (vm.Value, error) {
		if !vmInstance.IsConstructorCall() {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainMonthDay must be called with new")
		}
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainMonthDay requires month and day")
		}
		// Check for Infinity/NaN
		if err := checkTemporalArgs(vmInstance, args, []string{"month", "day", "calendar", "referenceISOYear"}); err != nil {
			return vm.Undefined, err
		}
		// Validate calendar if provided (3rd argument)
		if len(args) > 2 {
			if err := validateCalendarID(vmInstance, args[2]); err != nil {
				return vm.Undefined, err
			}
		}
		month := int(args[0].ToFloat())
		day := int(args[1].ToFloat())
		if month < 1 || month > 12 {
			return vm.Undefined, vmInstance.NewRangeError("Invalid month")
		}
		if day < 1 || day > 31 {
			return vm.Undefined, vmInstance.NewRangeError("Invalid day")
		}
		return createPlainMonthDay(month, day), nil
	})
	plainMonthDayCtorProps := plainMonthDayCtor.AsNativeFunctionWithProps()
	plainMonthDayCtorProps.Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(plainMonthDayProto), &w, &e, &c)

	plainMonthDayCtorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.PlainMonthDay.from requires an argument")
		}
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
		}
		item := args[0]
		if m, d, err := getPlainMonthDayFields(item); err == nil {
			return createPlainMonthDay(m, d), nil
		}
		if item.Type() == vm.TypeString {
			str := item.ToString()
			// Try MM-DD format
			if len(str) == 5 && str[2] == '-' {
				month, err1 := strconv.Atoi(str[:2])
				day, err2 := strconv.Atoi(str[3:])
				if err1 == nil && err2 == nil && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
					return createPlainMonthDay(month, day), nil
				}
			}
			// Try --MM-DD format
			if len(str) == 7 && str[0] == '-' && str[1] == '-' && str[4] == '-' {
				month, err1 := strconv.Atoi(str[2:4])
				day, err2 := strconv.Atoi(str[5:7])
				if err1 == nil && err2 == nil && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
					return createPlainMonthDay(month, day), nil
				}
			}
			// Try full date format
			t, err := time.Parse("2006-01-02", str)
			if err == nil {
				return createPlainMonthDay(int(t.Month()), t.Day()), nil
			}
			return vm.Undefined, vmInstance.NewRangeError("Invalid Temporal.PlainMonthDay string")
		}
		return vm.Undefined, vmInstance.NewTypeError("Invalid argument for Temporal.PlainMonthDay.from")
	}))

	plainMonthDayProto.SetOwnNonEnumerable("constructor", plainMonthDayCtor)
	temporalObj.SetOwnNonEnumerable("PlainMonthDay", plainMonthDayCtor)

	// ============================================
	// Create Temporal.Duration.prototype
	// ============================================
	durationProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		durationProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal.Duration"),
			&falseVal, &falseVal, &trueVal,
		)
	}

	getDurationFields := func(val vm.Value) (years, months, weeks, days, hours, minutes, seconds, ms, us, ns int, err error) {
		if !val.IsObject() {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.Duration")
		}
		obj := val.AsPlainObject()
		if obj == nil {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.Duration")
		}
		// Check for internal slots to verify this is a Temporal.Duration
		yearsVal, hasYears := obj.GetOwn("[[Years]]")
		if !hasYears {
			return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, vmInstance.NewTypeError("Value is not a Temporal.Duration")
		}
		monthsVal, _ := obj.GetOwn("[[Months]]")
		weeksVal, _ := obj.GetOwn("[[Weeks]]")
		daysVal, _ := obj.GetOwn("[[Days]]")
		hoursVal, _ := obj.GetOwn("[[Hours]]")
		minutesVal, _ := obj.GetOwn("[[Minutes]]")
		secondsVal, _ := obj.GetOwn("[[Seconds]]")
		msVal, _ := obj.GetOwn("[[Milliseconds]]")
		usVal, _ := obj.GetOwn("[[Microseconds]]")
		nsVal, _ := obj.GetOwn("[[Nanoseconds]]")
		return int(yearsVal.ToFloat()), int(monthsVal.ToFloat()), int(weeksVal.ToFloat()), int(daysVal.ToFloat()),
			int(hoursVal.ToFloat()), int(minutesVal.ToFloat()), int(secondsVal.ToFloat()),
			int(msVal.ToFloat()), int(usVal.ToFloat()), int(nsVal.ToFloat()), nil
	}

	createDuration := func(years, months, weeks, days, hours, minutes, seconds, ms, us, ns int) vm.Value {
		d := vm.NewObject(vm.NewValueFromPlainObject(durationProto)).AsPlainObject()
		d.SetOwn("[[Years]]", vm.NumberValue(float64(years)))
		d.SetOwn("[[Months]]", vm.NumberValue(float64(months)))
		d.SetOwn("[[Weeks]]", vm.NumberValue(float64(weeks)))
		d.SetOwn("[[Days]]", vm.NumberValue(float64(days)))
		d.SetOwn("[[Hours]]", vm.NumberValue(float64(hours)))
		d.SetOwn("[[Minutes]]", vm.NumberValue(float64(minutes)))
		d.SetOwn("[[Seconds]]", vm.NumberValue(float64(seconds)))
		d.SetOwn("[[Milliseconds]]", vm.NumberValue(float64(ms)))
		d.SetOwn("[[Microseconds]]", vm.NumberValue(float64(us)))
		d.SetOwn("[[Nanoseconds]]", vm.NumberValue(float64(ns)))
		return vm.NewValueFromPlainObject(d)
	}

	// Duration getters
	durationProto.DefineAccessorProperty("years", vm.NewNativeFunction(0, false, "get years", func(args []vm.Value) (vm.Value, error) {
		y, _, _, _, _, _, _, _, _, _, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(y)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("months", vm.NewNativeFunction(0, false, "get months", func(args []vm.Value) (vm.Value, error) {
		_, m, _, _, _, _, _, _, _, _, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(m)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("weeks", vm.NewNativeFunction(0, false, "get weeks", func(args []vm.Value) (vm.Value, error) {
		_, _, w, _, _, _, _, _, _, _, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(w)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("days", vm.NewNativeFunction(0, false, "get days", func(args []vm.Value) (vm.Value, error) {
		_, _, _, d, _, _, _, _, _, _, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(d)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("hours", vm.NewNativeFunction(0, false, "get hours", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, h, _, _, _, _, _, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(h)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("minutes", vm.NewNativeFunction(0, false, "get minutes", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, m, _, _, _, _, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(m)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("seconds", vm.NewNativeFunction(0, false, "get seconds", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, _, s, _, _, _, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(s)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("milliseconds", vm.NewNativeFunction(0, false, "get milliseconds", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, _, _, ms, _, _, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(ms)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("microseconds", vm.NewNativeFunction(0, false, "get microseconds", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, _, _, _, us, _, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(us)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("nanoseconds", vm.NewNativeFunction(0, false, "get nanoseconds", func(args []vm.Value) (vm.Value, error) {
		_, _, _, _, _, _, _, _, _, ns, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(ns)), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("sign", vm.NewNativeFunction(0, false, "get sign", func(args []vm.Value) (vm.Value, error) {
		y, mo, w, d, h, mi, s, ms, us, ns, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		// Check if all fields are zero
		if y == 0 && mo == 0 && w == 0 && d == 0 && h == 0 && mi == 0 && s == 0 && ms == 0 && us == 0 && ns == 0 {
			return vm.NumberValue(0), nil
		}
		// Check if any field is negative
		if y < 0 || mo < 0 || w < 0 || d < 0 || h < 0 || mi < 0 || s < 0 || ms < 0 || us < 0 || ns < 0 {
			return vm.NumberValue(-1), nil
		}
		return vm.NumberValue(1), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.DefineAccessorProperty("blank", vm.NewNativeFunction(0, false, "get blank", func(args []vm.Value) (vm.Value, error) {
		y, mo, w, d, h, mi, s, ms, us, ns, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		blank := y == 0 && mo == 0 && w == 0 && d == 0 && h == 0 && mi == 0 && s == 0 && ms == 0 && us == 0 && ns == 0
		return vm.BooleanValue(blank), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	durationProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		y, mo, w, d, h, mi, s, ms, us, ns, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		// ISO 8601 duration format: P[n]Y[n]M[n]W[n]DT[n]H[n]M[n]S
		result := "P"
		if y != 0 {
			result += fmt.Sprintf("%dY", y)
		}
		if mo != 0 {
			result += fmt.Sprintf("%dM", mo)
		}
		if w != 0 {
			result += fmt.Sprintf("%dW", w)
		}
		if d != 0 {
			result += fmt.Sprintf("%dD", d)
		}
		hasTime := h != 0 || mi != 0 || s != 0 || ms != 0 || us != 0 || ns != 0
		if hasTime {
			result += "T"
			if h != 0 {
				result += fmt.Sprintf("%dH", h)
			}
			if mi != 0 {
				result += fmt.Sprintf("%dM", mi)
			}
			if s != 0 || ms != 0 || us != 0 || ns != 0 {
				totalNano := ms*1000000 + us*1000 + ns
				if totalNano != 0 {
					fracStr := fmt.Sprintf(".%09d", totalNano)
					fracStr = strings.TrimRight(fracStr, "0")
					result += fmt.Sprintf("%d%sS", s, fracStr)
				} else {
					result += fmt.Sprintf("%dS", s)
				}
			}
		}
		if result == "P" {
			result = "PT0S"
		}
		return vm.NewString(result), nil
	}))

	durationProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		// Same as toString for Duration
		y, mo, w, d, h, mi, s, ms, us, ns, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		result := "P"
		if y != 0 {
			result += fmt.Sprintf("%dY", y)
		}
		if mo != 0 {
			result += fmt.Sprintf("%dM", mo)
		}
		if w != 0 {
			result += fmt.Sprintf("%dW", w)
		}
		if d != 0 {
			result += fmt.Sprintf("%dD", d)
		}
		hasTime := h != 0 || mi != 0 || s != 0 || ms != 0 || us != 0 || ns != 0
		if hasTime {
			result += "T"
			if h != 0 {
				result += fmt.Sprintf("%dH", h)
			}
			if mi != 0 {
				result += fmt.Sprintf("%dM", mi)
			}
			if s != 0 || ms != 0 || us != 0 || ns != 0 {
				totalNano := ms*1000000 + us*1000 + ns
				if totalNano != 0 {
					fracStr := fmt.Sprintf(".%09d", totalNano)
					fracStr = strings.TrimRight(fracStr, "0")
					result += fmt.Sprintf("%d%sS", s, fracStr)
				} else {
					result += fmt.Sprintf("%dS", s)
				}
			}
		}
		if result == "P" {
			result = "PT0S"
		}
		return vm.NewString(result), nil
	}))

	durationProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("use compare() to compare Temporal.Duration")
	}))

	durationProto.SetOwnNonEnumerable("negated", vm.NewNativeFunction(0, false, "negated", func(args []vm.Value) (vm.Value, error) {
		y, mo, w, d, h, mi, s, ms, us, ns, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return createDuration(-y, -mo, -w, -d, -h, -mi, -s, -ms, -us, -ns), nil
	}))

	durationProto.SetOwnNonEnumerable("abs", vm.NewNativeFunction(0, false, "abs", func(args []vm.Value) (vm.Value, error) {
		y, mo, w, d, h, mi, s, ms, us, ns, err := getDurationFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		abs := func(n int) int {
			if n < 0 {
				return -n
			}
			return n
		}
		return createDuration(abs(y), abs(mo), abs(w), abs(d), abs(h), abs(mi), abs(s), abs(ms), abs(us), abs(ns)), nil
	}))

	durationCtor := vm.NewConstructorWithProps(0, false, "Duration", func(args []vm.Value) (vm.Value, error) {
		if !vmInstance.IsConstructorCall() {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.Duration must be called with new")
		}
		// Duration field names for error messages
		durationArgNames := []string{"years", "months", "weeks", "days", "hours", "minutes", "seconds", "milliseconds", "microseconds", "nanoseconds"}

		// Convert each argument using ToIntegerIfIntegral (validates Symbol/BigInt, Infinity/NaN, and fractional)
		years, months, weeks, days, hours, minutes, seconds, ms, us, ns := 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
		var err error
		if len(args) > 0 {
			years, err = toIntegerIfIntegral(vmInstance, args[0], durationArgNames[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if len(args) > 1 {
			months, err = toIntegerIfIntegral(vmInstance, args[1], durationArgNames[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if len(args) > 2 {
			weeks, err = toIntegerIfIntegral(vmInstance, args[2], durationArgNames[2])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if len(args) > 3 {
			days, err = toIntegerIfIntegral(vmInstance, args[3], durationArgNames[3])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if len(args) > 4 {
			hours, err = toIntegerIfIntegral(vmInstance, args[4], durationArgNames[4])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if len(args) > 5 {
			minutes, err = toIntegerIfIntegral(vmInstance, args[5], durationArgNames[5])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if len(args) > 6 {
			seconds, err = toIntegerIfIntegral(vmInstance, args[6], durationArgNames[6])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if len(args) > 7 {
			ms, err = toIntegerIfIntegral(vmInstance, args[7], durationArgNames[7])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if len(args) > 8 {
			us, err = toIntegerIfIntegral(vmInstance, args[8], durationArgNames[8])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if len(args) > 9 {
			ns, err = toIntegerIfIntegral(vmInstance, args[9], durationArgNames[9])
			if err != nil {
				return vm.Undefined, err
			}
		}

		// Check for mixed signs - all non-zero values must have the same sign
		values := []int{years, months, weeks, days, hours, minutes, seconds, ms, us, ns}
		hasPositive, hasNegative := false, false
		for _, v := range values {
			if v > 0 {
				hasPositive = true
			} else if v < 0 {
				hasNegative = true
			}
		}
		if hasPositive && hasNegative {
			return vm.Undefined, vmInstance.NewRangeError("Duration fields must not have mixed signs")
		}

		return createDuration(years, months, weeks, days, hours, minutes, seconds, ms, us, ns), nil
	})
	durationCtorProps := durationCtor.AsNativeFunctionWithProps()
	durationCtorProps.Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(durationProto), &w, &e, &c)

	durationCtorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.Duration.from requires an argument")
		}
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
		}
		item := args[0]
		// First try as existing Temporal.Duration object
		if y, mo, w, d, h, mi, s, ms, us, ns, err := getDurationFields(item); err == nil {
			return createDuration(y, mo, w, d, h, mi, s, ms, us, ns), nil
		}
		// Then try as string
		if item.Type() == vm.TypeString {
			return parseDurationString(vmInstance, item.ToString(), createDuration)
		}
		// Finally try as duration-like object (with type validation for Symbol/BigInt)
		if y, mo, w, d, h, mi, s, ms, us, ns, err := getDurationFromArg(item); err == nil {
			return createDuration(y, mo, w, d, h, mi, s, ms, us, ns), nil
		} else {
			return vm.Undefined, err
		}
	}))

	durationCtorProps.Properties.SetOwnNonEnumerable("compare", vm.NewNativeFunction(2, false, "compare", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("compare requires two arguments")
		}
		// Validate options if provided
		if len(args) > 2 {
			if err := validateTemporalOptions(vmInstance, args[2]); err != nil {
				return vm.Undefined, err
			}
		}
		// For durations without relativeTo, we can only compare time-based durations
		y1, mo1, w1, d1, h1, mi1, s1, ms1, us1, ns1, err := getDurationFields(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		y2, mo2, w2, d2, h2, mi2, s2, ms2, us2, ns2, err := getDurationFields(args[1])
		if err != nil {
			return vm.Undefined, err
		}
		// Convert to total nanoseconds for comparison (simplified)
		total1 := int64(y1)*365*24*3600*1e9 + int64(mo1)*30*24*3600*1e9 + int64(w1)*7*24*3600*1e9 +
			int64(d1)*24*3600*1e9 + int64(h1)*3600*1e9 + int64(mi1)*60*1e9 + int64(s1)*1e9 +
			int64(ms1)*1e6 + int64(us1)*1e3 + int64(ns1)
		total2 := int64(y2)*365*24*3600*1e9 + int64(mo2)*30*24*3600*1e9 + int64(w2)*7*24*3600*1e9 +
			int64(d2)*24*3600*1e9 + int64(h2)*3600*1e9 + int64(mi2)*60*1e9 + int64(s2)*1e9 +
			int64(ms2)*1e6 + int64(us2)*1e3 + int64(ns2)
		if total1 < total2 {
			return vm.NumberValue(-1), nil
		}
		if total1 > total2 {
			return vm.NumberValue(1), nil
		}
		return vm.NumberValue(0), nil
	}))

	durationProto.SetOwnNonEnumerable("constructor", durationCtor)
	temporalObj.SetOwnNonEnumerable("Duration", durationCtor)

	// ============================================
	// Create Temporal.ZonedDateTime.prototype
	// ============================================
	zonedDateTimeProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		zonedDateTimeProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Temporal.ZonedDateTime"),
			&falseVal, &falseVal, &trueVal,
		)
	}

	getZonedDateTimeFields := func(val vm.Value) (nanos *big.Int, tzId string, err error) {
		if !val.IsObject() {
			return nil, "", vmInstance.NewTypeError("Value is not a Temporal.ZonedDateTime")
		}
		obj := val.AsPlainObject()
		if obj == nil {
			return nil, "", vmInstance.NewTypeError("Value is not a Temporal.ZonedDateTime")
		}
		nanosVal, nanosOk := obj.GetOwn("[[EpochNanoseconds]]")
		tzVal, tzOk := obj.GetOwn("[[TimeZone]]")
		if !nanosOk || !tzOk {
			return nil, "", vmInstance.NewTypeError("Value is not a Temporal.ZonedDateTime")
		}
		if nanosVal.Type() == vm.TypeBigInt {
			nanos = nanosVal.AsBigInt()
		} else {
			nanos = new(big.Int).SetInt64(int64(nanosVal.ToFloat()))
		}
		return nanos, tzVal.ToString(), nil
	}

	createZonedDateTime := func(nanos *big.Int, tzId string) vm.Value {
		zdt := vm.NewObject(vm.NewValueFromPlainObject(zonedDateTimeProto)).AsPlainObject()
		zdt.SetOwn("[[EpochNanoseconds]]", vm.NewBigInt(nanos))
		zdt.SetOwn("[[TimeZone]]", vm.NewString(tzId))
		zdt.SetOwn("[[Calendar]]", vm.NewString("iso8601"))
		return vm.NewValueFromPlainObject(zdt)
	}

	// Helper to convert nanoseconds to time.Time in a timezone
	nanosToTimeInZone := func(nanos *big.Int, tzId string) time.Time {
		secs := new(big.Int).Div(nanos, big.NewInt(1e9)).Int64()
		nsec := new(big.Int).Mod(nanos, big.NewInt(1e9)).Int64()
		t := time.Unix(secs, nsec)
		if loc, err := time.LoadLocation(tzId); err == nil {
			t = t.In(loc)
		}
		return t
	}

	// ZonedDateTime getters
	zonedDateTimeProto.DefineAccessorProperty("epochNanoseconds", vm.NewNativeFunction(0, false, "get epochNanoseconds", func(args []vm.Value) (vm.Value, error) {
		nanos, _, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewBigInt(nanos), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("epochMilliseconds", vm.NewNativeFunction(0, false, "get epochMilliseconds", func(args []vm.Value) (vm.Value, error) {
		nanos, _, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		ms := new(big.Int).Div(nanos, big.NewInt(1e6))
		return vm.NumberValue(float64(ms.Int64())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("timeZoneId", vm.NewNativeFunction(0, false, "get timeZoneId", func(args []vm.Value) (vm.Value, error) {
		_, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(tzId), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("calendarId", vm.NewNativeFunction(0, false, "get calendarId", func(args []vm.Value) (vm.Value, error) {
		return vm.NewString("iso8601"), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("year", vm.NewNativeFunction(0, false, "get year", func(args []vm.Value) (vm.Value, error) {
		nanos, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := nanosToTimeInZone(nanos, tzId)
		return vm.NumberValue(float64(t.Year())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("month", vm.NewNativeFunction(0, false, "get month", func(args []vm.Value) (vm.Value, error) {
		nanos, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := nanosToTimeInZone(nanos, tzId)
		return vm.NumberValue(float64(t.Month())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("day", vm.NewNativeFunction(0, false, "get day", func(args []vm.Value) (vm.Value, error) {
		nanos, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := nanosToTimeInZone(nanos, tzId)
		return vm.NumberValue(float64(t.Day())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("hour", vm.NewNativeFunction(0, false, "get hour", func(args []vm.Value) (vm.Value, error) {
		nanos, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := nanosToTimeInZone(nanos, tzId)
		return vm.NumberValue(float64(t.Hour())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("minute", vm.NewNativeFunction(0, false, "get minute", func(args []vm.Value) (vm.Value, error) {
		nanos, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := nanosToTimeInZone(nanos, tzId)
		return vm.NumberValue(float64(t.Minute())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("second", vm.NewNativeFunction(0, false, "get second", func(args []vm.Value) (vm.Value, error) {
		nanos, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := nanosToTimeInZone(nanos, tzId)
		return vm.NumberValue(float64(t.Second())), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.DefineAccessorProperty("hoursInDay", vm.NewNativeFunction(0, false, "get hoursInDay", func(args []vm.Value) (vm.Value, error) {
		// Most days have 24 hours, but DST transitions can have 23 or 25
		nanos, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := nanosToTimeInZone(nanos, tzId)
		loc := t.Location()
		startOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		startOfNextDay := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
		hours := startOfNextDay.Sub(startOfDay).Hours()
		return vm.NumberValue(hours), nil
	}), true, vm.Undefined, false, &falseVal, &trueVal)

	zonedDateTimeProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(1, false, "toString", func(args []vm.Value) (vm.Value, error) {
		nanos, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		// Validate options if provided
		if len(args) > 0 {
			if err := validateTemporalOptions(vmInstance, args[0]); err != nil {
				return vm.Undefined, err
			}
		}
		t := nanosToTimeInZone(nanos, tzId)
		_, offset := t.Zone()
		offsetStr := formatOffset(offset)

		// Check for options
		timeZoneName := "auto" // default
		if len(args) > 0 && args[0].IsObject() {
			opts := args[0].AsPlainObject()
			if opts != nil {
				if tzNameVal, ok := opts.Get("timeZoneName"); ok && tzNameVal.Type() == vm.TypeString {
					timeZoneName = tzNameVal.ToString()
				}
			}
		}

		// Format time with fractional seconds if needed
		timeStr := fmt.Sprintf("%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
		nsec := t.Nanosecond()
		if nsec > 0 {
			fracStr := fmt.Sprintf(".%09d", nsec)
			fracStr = strings.TrimRight(fracStr, "0")
			timeStr += fracStr
		}

		var result string
		switch timeZoneName {
		case "never":
			// Don't include time zone annotation
			result = fmt.Sprintf("%04d-%02d-%02dT%s%s",
				t.Year(), t.Month(), t.Day(), timeStr, offsetStr)
		case "critical":
			// Include time zone with critical flag
			result = fmt.Sprintf("%04d-%02d-%02dT%s%s[!%s]",
				t.Year(), t.Month(), t.Day(), timeStr, offsetStr, tzId)
		default: // "auto" or any other value
			result = fmt.Sprintf("%04d-%02d-%02dT%s%s[%s]",
				t.Year(), t.Month(), t.Day(), timeStr, offsetStr, tzId)
		}
		return vm.NewString(result), nil
	}))

	zonedDateTimeProto.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", func(args []vm.Value) (vm.Value, error) {
		nanos, tzId, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		t := nanosToTimeInZone(nanos, tzId)
		_, offset := t.Zone()
		offsetStr := formatOffset(offset)
		timeStr := fmt.Sprintf("%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
		nsec := t.Nanosecond()
		if nsec > 0 {
			fracStr := fmt.Sprintf(".%09d", nsec)
			fracStr = strings.TrimRight(fracStr, "0")
			timeStr += fracStr
		}
		result := fmt.Sprintf("%04d-%02d-%02dT%s%s[%s]",
			t.Year(), t.Month(), t.Day(), timeStr, offsetStr, tzId)
		return vm.NewString(result), nil
	}))

	zonedDateTimeProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("use compare() to compare Temporal.ZonedDateTime")
	}))

	// Temporal.ZonedDateTime.prototype.equals(other)
	zonedDateTimeProto.SetOwnNonEnumerable("equals", vm.NewNativeFunction(1, false, "equals", func(args []vm.Value) (vm.Value, error) {
		thisNanos, thisTz, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("equals requires an argument")
		}
		otherNanos, otherTz, err := getZonedDateTimeFields(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// Both epochNanoseconds and timeZone must be equal
		return vm.BooleanValue(thisNanos.Cmp(otherNanos) == 0 && thisTz == otherTz), nil
	}))

	// Temporal.ZonedDateTime.prototype.since(other [, options])
	zonedDateTimeProto.SetOwnNonEnumerable("since", vm.NewNativeFunction(1, false, "since", func(args []vm.Value) (vm.Value, error) {
		thisNanos, _, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("since requires an argument")
		}
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
		}
		otherNanos, _, err := getZonedDateTimeFields(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// If equal, return blank duration
		if thisNanos.Cmp(otherNanos) == 0 {
			return createDuration(0, 0, 0, 0, 0, 0, 0, 0, 0, 0), nil
		}
		// Calculate difference in nanoseconds
		diff := new(big.Int).Sub(thisNanos, otherNanos)
		// Convert to duration components (simplified - just time-based)
		ns := diff.Int64()
		hours := int(ns / (3600 * 1e9))
		ns %= 3600 * 1e9
		minutes := int(ns / (60 * 1e9))
		ns %= 60 * 1e9
		seconds := int(ns / 1e9)
		ns %= 1e9
		msVal := int(ns / 1e6)
		ns %= 1e6
		usVal := int(ns / 1e3)
		nsVal := int(ns % 1e3)
		return createDuration(0, 0, 0, 0, hours, minutes, seconds, msVal, usVal, nsVal), nil
	}))

	// Temporal.ZonedDateTime.prototype.until(other [, options])
	zonedDateTimeProto.SetOwnNonEnumerable("until", vm.NewNativeFunction(1, false, "until", func(args []vm.Value) (vm.Value, error) {
		thisNanos, _, err := getZonedDateTimeFields(vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("until requires an argument")
		}
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
		}
		otherNanos, _, err := getZonedDateTimeFields(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		// If equal, return blank duration
		if thisNanos.Cmp(otherNanos) == 0 {
			return createDuration(0, 0, 0, 0, 0, 0, 0, 0, 0, 0), nil
		}
		// Calculate difference in nanoseconds (until is reverse of since)
		diff := new(big.Int).Sub(otherNanos, thisNanos)
		// Convert to duration components (simplified - just time-based)
		ns := diff.Int64()
		hours := int(ns / (3600 * 1e9))
		ns %= 3600 * 1e9
		minutes := int(ns / (60 * 1e9))
		ns %= 60 * 1e9
		seconds := int(ns / 1e9)
		ns %= 1e9
		msVal := int(ns / 1e6)
		ns %= 1e6
		usVal := int(ns / 1e3)
		nsVal := int(ns % 1e3)
		return createDuration(0, 0, 0, 0, hours, minutes, seconds, msVal, usVal, nsVal), nil
	}))

	zonedDateTimeCtor := vm.NewConstructorWithProps(2, false, "ZonedDateTime", func(args []vm.Value) (vm.Value, error) {
		if !vmInstance.IsConstructorCall() {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.ZonedDateTime must be called with new")
		}
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.ZonedDateTime requires epochNanoseconds and timeZone")
		}
		var nanos *big.Int
		if args[0].Type() == vm.TypeBigInt {
			nanos = args[0].AsBigInt()
		} else {
			nanos = new(big.Int).SetInt64(int64(args[0].ToFloat()))
		}
		tzId := args[1].ToString()
		// Validate timezone
		if _, err := time.LoadLocation(tzId); err != nil {
			// Try as UTC offset format
			if !strings.HasPrefix(tzId, "+") && !strings.HasPrefix(tzId, "-") && tzId != "UTC" {
				return vm.Undefined, vmInstance.NewRangeError("Invalid time zone: " + tzId)
			}
		}
		// Validate calendar if provided (3rd argument)
		if len(args) > 2 {
			if err := validateCalendarID(vmInstance, args[2]); err != nil {
				return vm.Undefined, err
			}
		}
		return createZonedDateTime(nanos, tzId), nil
	})
	zonedDateTimeCtorProps := zonedDateTimeCtor.AsNativeFunctionWithProps()
	zonedDateTimeCtorProps.Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(zonedDateTimeProto), &w, &e, &c)

	zonedDateTimeCtorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Temporal.ZonedDateTime.from requires an argument")
		}
		// Validate options if provided
		if len(args) > 1 {
			if err := validateTemporalOptions(vmInstance, args[1]); err != nil {
				return vm.Undefined, err
			}
		}
		item := args[0]
		if nanos, tzId, err := getZonedDateTimeFields(item); err == nil {
			return createZonedDateTime(nanos, tzId), nil
		}
		if item.Type() == vm.TypeString {
			return parseZonedDateTimeString(vmInstance, item.ToString(), createZonedDateTime)
		}
		return vm.Undefined, vmInstance.NewTypeError("Invalid argument for Temporal.ZonedDateTime.from")
	}))

	zonedDateTimeCtorProps.Properties.SetOwnNonEnumerable("compare", vm.NewNativeFunction(2, false, "compare", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("compare requires two arguments")
		}
		nanos1, _, err := getZonedDateTimeFields(args[0])
		if err != nil {
			return vm.Undefined, err
		}
		nanos2, _, err := getZonedDateTimeFields(args[1])
		if err != nil {
			return vm.Undefined, err
		}
		cmp := nanos1.Cmp(nanos2)
		return vm.NumberValue(float64(cmp)), nil
	}))

	zonedDateTimeProto.SetOwnNonEnumerable("constructor", zonedDateTimeCtor)
	temporalObj.SetOwnNonEnumerable("ZonedDateTime", zonedDateTimeCtor)

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

// isFiniteNumber checks if a float64 is a finite number (not Infinity or NaN)
func isFiniteNumber(f float64) bool {
	return !math.IsInf(f, 0) && !math.IsNaN(f)
}

// checkTemporalArgs validates that all arguments are finite numbers and returns a RangeError if not
func checkTemporalArgs(vmInstance *vm.VM, args []vm.Value, names []string) error {
	for i, arg := range args {
		if i < len(names) && arg.IsNumber() {
			f := arg.ToFloat()
			if !isFiniteNumber(f) {
				return vmInstance.NewRangeError(names[i] + " must be a finite number")
			}
		}
	}
	return nil
}

// validateTemporalOptions validates that the options argument is either undefined or an object
// Returns an error for null, boolean, string, number, bigint, symbol
func validateTemporalOptions(vmInstance *vm.VM, options vm.Value) error {
	if options.Type() == vm.TypeUndefined {
		return nil
	}
	// null is explicitly not allowed (it's a special case, not an object for this purpose)
	if options.Type() == vm.TypeNull {
		return vmInstance.NewTypeError("options must be an object")
	}
	// Objects and functions are allowed (functions are objects in JavaScript)
	if !options.IsObject() && !options.IsCallable() {
		return vmInstance.NewTypeError("options must be an object")
	}
	return nil
}

// validateCalendarID validates a calendar ID string
// Only "iso8601" is currently supported. Invalid strings throw RangeError.
func validateCalendarID(vmInstance *vm.VM, calendarArg vm.Value) error {
	// undefined means use default "iso8601" calendar
	if calendarArg.Type() == vm.TypeUndefined {
		return nil
	}
	// Must be a string
	if calendarArg.Type() != vm.TypeString {
		return vmInstance.NewTypeError("calendar must be a string")
	}
	calendar := calendarArg.ToString()
	// Empty string is invalid
	if calendar == "" {
		return vmInstance.NewRangeError("Invalid calendar: empty string")
	}
	// ISO string with calendar annotation is invalid (contains brackets)
	if strings.Contains(calendar, "[") || strings.Contains(calendar, "]") {
		return vmInstance.NewRangeError("Invalid calendar: cannot use ISO string with calendar annotation")
	}
	// Only iso8601 is supported for now
	if calendar != "iso8601" {
		return vmInstance.NewRangeError("Invalid calendar: " + calendar)
	}
	return nil
}

// toIntegerIfIntegral converts a value to an integer, throwing TypeError for Symbol and BigInt
// This implements the ToIntegerIfIntegral abstract operation from the Temporal spec
func toIntegerIfIntegral(vmInstance *vm.VM, val vm.Value, name string) (int, error) {
	// undefined becomes 0
	if val.Type() == vm.TypeUndefined {
		return 0, nil
	}
	// Symbol and BigInt cannot be converted to number
	if val.Type() == vm.TypeSymbol {
		return 0, vmInstance.NewTypeError("Cannot convert Symbol to number")
	}
	if val.Type() == vm.TypeBigInt {
		return 0, vmInstance.NewTypeError("Cannot convert BigInt to number")
	}
	// Convert to number
	f := val.ToFloat()
	// Check for NaN and Infinity
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, vmInstance.NewRangeError(name + " must be a finite number")
	}
	// Check for non-integer
	if f != math.Trunc(f) {
		return 0, vmInstance.NewRangeError(name + " must be an integer")
	}
	return int(f), nil
}

// getOverflowOption reads and validates the overflow option from an options object
// Returns "constrain" (default), "reject", or an error for invalid values
// This reads the option FIRST before any algorithmic validation per ECMAScript spec
func getOverflowOption(vmInstance *vm.VM, options vm.Value) (string, error) {
	if options.Type() == vm.TypeUndefined {
		return "constrain", nil
	}
	if !options.IsObject() && !options.IsCallable() {
		return "", vmInstance.NewTypeError("options must be an object")
	}
	// Get the overflow property using GetProperty which handles any object type
	overflowVal, err := vmInstance.GetProperty(options, "overflow")
	if err != nil {
		return "", err
	}
	if overflowVal.Type() == vm.TypeUndefined {
		return "constrain", nil
	}
	// Convert to string
	overflow := overflowVal.ToString()
	if overflow != "constrain" && overflow != "reject" {
		return "", vmInstance.NewRangeError("Invalid overflow option: " + overflow)
	}
	return overflow, nil
}

// daysInMonth returns the number of days in a given month/year
func daysInMonth(year, month int) int {
	// Use time.Date to compute days in month by going to next month and subtracting
	return time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
}

// addDateWithOverflow adds duration to a date, respecting the overflow option
// Returns the new date or an error if overflow="reject" and date would be clamped
func addDateWithOverflow(vmInstance *vm.VM, y, m, d, years, months, days int, overflow string) (int, int, int, error) {
	// First add years and months
	newYear := y + years
	newMonth := m + months
	// Normalize months
	for newMonth > 12 {
		newYear++
		newMonth -= 12
	}
	for newMonth < 1 {
		newYear--
		newMonth += 12
	}
	// Check if day is valid for new month
	maxDay := daysInMonth(newYear, newMonth)
	newDay := d
	if d > maxDay {
		if overflow == "reject" {
			return 0, 0, 0, vmInstance.NewRangeError("day is out of range for month")
		}
		// Constrain to last day of month
		newDay = maxDay
	}
	// Now add days using time.Date for proper handling
	t := time.Date(newYear, time.Month(newMonth), newDay, 0, 0, 0, 0, time.UTC)
	t = t.AddDate(0, 0, days)
	return t.Year(), int(t.Month()), t.Day(), nil
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

// formatOffset formats a timezone offset in seconds to +HH:MM or -HH:MM format
func formatOffset(offsetSeconds int) string {
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60
	return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
}

// parseDurationString parses an ISO 8601 duration string (e.g., "P1Y2M3DT4H5M6S")
func parseDurationString(vmInstance *vm.VM, str string, createDuration func(int, int, int, int, int, int, int, int, int, int) vm.Value) (vm.Value, error) {
	if len(str) == 0 || str[0] != 'P' {
		return vm.Undefined, vmInstance.NewRangeError("Invalid Temporal.Duration string: must start with P")
	}

	years, months, weeks, days := 0, 0, 0, 0
	hours, minutes, seconds := 0, 0, 0
	milliseconds, microseconds, nanoseconds := 0, 0, 0

	str = str[1:] // Remove 'P'
	inTimePart := false
	numStr := ""

	for i := 0; i < len(str); i++ {
		c := str[i]
		if c >= '0' && c <= '9' || c == '.' || c == '-' {
			numStr += string(c)
		} else if c == 'T' {
			inTimePart = true
			numStr = ""
		} else {
			if numStr == "" {
				continue
			}
			// Parse the number
			var num float64
			if _, err := fmt.Sscanf(numStr, "%f", &num); err != nil {
				return vm.Undefined, vmInstance.NewRangeError("Invalid duration string: cannot parse number")
			}
			numStr = ""

			if !inTimePart {
				switch c {
				case 'Y':
					years = int(num)
				case 'M':
					months = int(num)
				case 'W':
					weeks = int(num)
				case 'D':
					days = int(num)
				}
			} else {
				switch c {
				case 'H':
					hours = int(num)
				case 'M':
					minutes = int(num)
				case 'S':
					// Handle fractional seconds
					intPart := int(num)
					fracPart := num - float64(intPart)
					seconds = intPart
					if fracPart > 0 {
						totalNanos := int(fracPart * 1e9)
						milliseconds = totalNanos / 1000000
						microseconds = (totalNanos / 1000) % 1000
						nanoseconds = totalNanos % 1000
					}
				}
			}
		}
	}

	return createDuration(years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds), nil
}

// parseZonedDateTimeString parses a ZonedDateTime string (e.g., "2024-01-15T10:30:00+05:30[Asia/Kolkata]")
func parseZonedDateTimeString(vmInstance *vm.VM, str string, createZonedDateTime func(*big.Int, string) vm.Value) (vm.Value, error) {
	// Extract timezone annotation if present [TimeZone]
	tzId := ""
	if bracketStart := strings.Index(str, "["); bracketStart > 0 {
		if bracketEnd := strings.Index(str[bracketStart:], "]"); bracketEnd > 0 {
			tzId = str[bracketStart+1 : bracketStart+bracketEnd]
			// Remove the annotation from the string for parsing
			str = str[:bracketStart]
		}
	}

	// Try to parse as instant
	layouts := []string{
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.999999999Z",
		"2006-01-02T15:04:05Z",
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
		// Try to handle +HH:MM format manually
		if idx := strings.LastIndex(str, "+"); idx > 0 {
			base := str[:idx]
			offset := str[idx:]
			if len(offset) == 6 && offset[3] == ':' {
				hours, _ := strconv.Atoi(offset[1:3])
				mins, _ := strconv.Atoi(offset[4:6])
				totalMins := hours*60 + mins
				loc := time.FixedZone("", totalMins*60)
				for _, baseLayout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
					if t, err = time.ParseInLocation(baseLayout, base, loc); err == nil {
						parsed = true
						if tzId == "" {
							tzId = offset
						}
						break
					}
				}
			}
		} else if idx := strings.LastIndex(str, "-"); idx > 10 {
			base := str[:idx]
			offset := str[idx:]
			if len(offset) == 6 && offset[3] == ':' {
				hours, _ := strconv.Atoi(offset[1:3])
				mins, _ := strconv.Atoi(offset[4:6])
				totalMins := -(hours*60 + mins)
				loc := time.FixedZone("", totalMins*60)
				for _, baseLayout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
					if t, err = time.ParseInLocation(baseLayout, base, loc); err == nil {
						parsed = true
						if tzId == "" {
							tzId = offset
						}
						break
					}
				}
			}
		}
	}

	if !parsed {
		return vm.Undefined, vmInstance.NewRangeError("Invalid Temporal.ZonedDateTime string: " + str)
	}

	// If no timezone annotation, use the offset from the string
	if tzId == "" {
		_, offset := t.Zone()
		if offset == 0 {
			tzId = "UTC"
		} else {
			tzId = formatOffset(offset)
		}
	}

	nanos := new(big.Int).SetInt64(t.UTC().UnixNano())
	return createZonedDateTime(nanos, tzId), nil
}

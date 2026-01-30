package builtins

import (
	"math"
	"strconv"
	"strings"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// formatExponent removes leading zeros from exponent part to match JS behavior
// e.g., "1e+02" -> "1e+2", "1.5e-09" -> "1.5e-9"
func formatExponent(s string) string {
	// Find 'e' or 'E'
	eIdx := strings.IndexByte(s, 'e')
	if eIdx < 0 {
		eIdx = strings.IndexByte(s, 'E')
	}
	if eIdx < 0 || eIdx >= len(s)-1 {
		return s
	}

	// Get mantissa and exponent parts
	mantissa := s[:eIdx+1] // includes 'e' or 'E'
	expPart := s[eIdx+1:]

	// Handle sign
	sign := ""
	if len(expPart) > 0 && (expPart[0] == '+' || expPart[0] == '-') {
		sign = string(expPart[0])
		expPart = expPart[1:]
	}

	// Skip leading zeros but keep at least one digit
	i := 0
	for i < len(expPart)-1 && expPart[i] == '0' {
		i++
	}
	expPart = expPart[i:]

	return mantissa + sign + expPart
}

// formatToPrecision formats a number with the given precision, matching JS behavior
func formatToPrecision(num float64, precision int) string {
	if num == 0 {
		// Special case for zero
		if precision == 1 {
			return "0"
		}
		return "0." + strings.Repeat("0", precision-1)
	}

	// Get the exponent
	absNum := math.Abs(num)
	var exp int
	if absNum != 0 {
		exp = int(math.Floor(math.Log10(absNum)))
	}

	// Decide between fixed and exponential notation
	// ECMAScript spec: use exponential if exp < -6 or exp >= precision
	if exp < -6 || exp >= precision {
		// Exponential notation
		result := strconv.FormatFloat(num, 'e', precision-1, 64)
		return formatExponent(result)
	}

	// Fixed notation
	// Number of decimal places needed
	decimalPlaces := precision - exp - 1
	if decimalPlaces < 0 {
		decimalPlaces = 0
	}

	result := strconv.FormatFloat(num, 'f', decimalPlaces, 64)

	// Ensure we have exactly 'precision' significant digits
	// Count current significant digits
	sigDigits := 0
	foundNonZero := false
	hasDecimal := strings.Contains(result, ".")

	for _, c := range result {
		if c == '-' || c == '.' {
			continue
		}
		if c != '0' {
			foundNonZero = true
		}
		if foundNonZero {
			sigDigits++
		}
	}

	// Add trailing zeros if needed
	if sigDigits < precision {
		needed := precision - sigDigits
		if !hasDecimal {
			result += "."
		}
		result += strings.Repeat("0", needed)
	}

	return result
}

type NumberInitializer struct{}

func (n *NumberInitializer) Name() string {
	return "Number"
}

func (n *NumberInitializer) Priority() int {
	return 350 // After String (300)
}

func (n *NumberInitializer) InitTypes(ctx *TypeContext) error {
	// Create Number constructor type first (needed for constructor property)
	numberCtorType := types.NewSimpleFunction([]types.Type{types.Any}, types.Number).
		WithProperty("MAX_VALUE", types.Number).
		WithProperty("MIN_VALUE", types.Number).
		WithProperty("NaN", types.Number).
		WithProperty("NEGATIVE_INFINITY", types.Number).
		WithProperty("POSITIVE_INFINITY", types.Number).
		WithProperty("MAX_SAFE_INTEGER", types.Number).
		WithProperty("MIN_SAFE_INTEGER", types.Number).
		WithProperty("EPSILON", types.Number).
		WithProperty("isNaN", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("isFinite", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("isInteger", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("isSafeInteger", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("parseFloat", types.NewSimpleFunction([]types.Type{types.String}, types.Number)).
		WithProperty("parseInt", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Number, []bool{false, true}))

	// Create Number.prototype type with all methods
	// Note: 'this' is implicit and not included in type signatures
	numberProtoType := types.NewObjectType().
		WithProperty("toString", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("toLocaleString", types.NewOptionalFunction([]types.Type{types.String, types.Any}, types.String, []bool{true, true})).
		WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("toFixed", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("toExponential", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("toPrecision", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("constructor", types.Any) // Avoid circular reference, use Any for constructor property

	// Register number primitive prototype
	ctx.SetPrimitivePrototype("number", numberProtoType)

	// Add prototype property to constructor
	numberCtorType = numberCtorType.WithProperty("prototype", numberProtoType)

	// Define Number constructor in global environment
	return ctx.DefineGlobal("Number", numberCtorType)
}

func (n *NumberInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Number.prototype as a Number object with [[PrimitiveValue]] = 0
	// Per ECMAScript spec, Number.prototype is a Number object whose [[NumberData]] is +0
	numberProto := vm.NewObject(objectProto).AsPlainObject()
	numberProto.SetOwn("[[PrimitiveValue]]", vm.NumberValue(0))

	// Add Number prototype methods
	numberProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(1, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisNum := vmInstance.GetThis()

		// Extract primitive value from Number wrapper object
		if thisNum.IsObject() {
			obj := thisNum.AsPlainObject()
			if primVal, found := obj.GetOwn("[[PrimitiveValue]]"); found && primVal != vm.Undefined {
				thisNum = primVal
			}
		}

		// Check if this is a number
		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber {
			// Try to convert or throw error
			if thisNum.Type() == vm.TypeBigInt {
				return vm.NewString(thisNum.ToString()), nil
			}
			// For non-numbers, throw a TypeError
			return vm.Undefined, vmInstance.NewTypeError("Number.prototype.toString requires that 'this' be a Number")
		}

		var radix int = 10
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			r := args[0].ToFloat()
			// ToInteger per spec (truncate to integer)
			if math.IsNaN(r) {
				r = 0
			}
			radix = int(r)
			if radix < 2 || radix > 36 {
				return vm.Undefined, vmInstance.NewRangeError("toString() radix must be between 2 and 36")
			}
		}

		numVal := thisNum.ToFloat()

		// Handle special cases - NaN and Infinity always return their string form regardless of radix
		if math.IsNaN(numVal) {
			return vm.NewString("NaN"), nil
		}
		if math.IsInf(numVal, 1) {
			return vm.NewString("Infinity"), nil
		}
		if math.IsInf(numVal, -1) {
			return vm.NewString("-Infinity"), nil
		}

		if radix == 10 {
			return vm.NewString(thisNum.ToString()), nil
		}

		// Handle different radix - only for finite numbers
		if thisNum.Type() == vm.TypeIntegerNumber {
			return vm.NewString(strconv.FormatInt(int64(thisNum.AsInteger()), radix)), nil
		} else {
			// For float numbers with non-10 radix, convert to int first (JS behavior)
			intVal := int64(numVal)
			return vm.NewString(strconv.FormatInt(intVal, radix)), nil
		}
	}))

	numberProto.SetOwnNonEnumerable("toLocaleString", vm.NewNativeFunction(2, false, "toLocaleString", func(args []vm.Value) (vm.Value, error) {
		thisNum := vmInstance.GetThis()

		// Check if this is a number
		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber && thisNum.Type() != vm.TypeBigInt {
			return vm.NewString(thisNum.ToString()), nil
		}

		// For now, just return the string representation (proper locale support would be complex)
		// TODO: Implement proper locale formatting
		return vm.NewString(thisNum.ToString()), nil
	}))

	numberProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		thisNum := vmInstance.GetThis()

		// If this is a primitive number, return it
		if thisNum.Type() == vm.TypeFloatNumber || thisNum.Type() == vm.TypeIntegerNumber {
			return thisNum, nil
		}

		// If this is a Number wrapper object, extract [[PrimitiveValue]]
		if thisNum.IsObject() {
			if primitiveVal, exists := thisNum.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				return primitiveVal, nil
			}
		}

		// TypeError: Number.prototype.valueOf requires that 'this' be a Number
		return vm.Undefined, vmInstance.NewTypeError("Number.prototype.valueOf requires that 'this' be a Number")
	}))

	numberProto.SetOwnNonEnumerable("toFixed", vm.NewNativeFunction(1, false, "toFixed", func(args []vm.Value) (vm.Value, error) {
		thisNum := vmInstance.GetThis()

		// Extract primitive value from wrapper if needed
		if thisNum.IsObject() {
			if primitiveVal, exists := thisNum.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				thisNum = primitiveVal
			}
		}

		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber {
			return vm.Undefined, vmInstance.NewTypeError("Number.prototype.toFixed requires that 'this' be a Number")
		}

		// ToInteger the fractionDigits (default 0)
		digits := 0
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			fd := args[0].ToFloat()
			if math.IsNaN(fd) {
				fd = 0
			}
			digits = int(fd)
		}

		// RangeError if fractionDigits is out of range
		if digits < 0 || digits > 100 {
			return vm.Undefined, vmInstance.NewRangeError("toFixed() digits argument must be between 0 and 100")
		}

		numVal := thisNum.ToFloat()

		// Handle special cases
		if math.IsNaN(numVal) {
			return vm.NewString("NaN"), nil
		}
		if math.IsInf(numVal, 1) {
			return vm.NewString("Infinity"), nil
		}
		if math.IsInf(numVal, -1) {
			return vm.NewString("-Infinity"), nil
		}

		// For very large numbers (>= 10^21), return exponential notation via ToString
		if math.Abs(numVal) >= 1e21 {
			return vm.NewString(strconv.FormatFloat(numVal, 'g', -1, 64)), nil
		}

		return vm.NewString(strconv.FormatFloat(numVal, 'f', digits, 64)), nil
	}))

	numberProto.SetOwnNonEnumerable("toExponential", vm.NewNativeFunction(1, false, "toExponential", func(args []vm.Value) (vm.Value, error) {
		thisNum := vmInstance.GetThis()

		// Extract primitive value from wrapper if needed
		if thisNum.IsObject() {
			if primitiveVal, exists := thisNum.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				thisNum = primitiveVal
			}
		}

		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber {
			return vm.Undefined, vmInstance.NewTypeError("Number.prototype.toExponential requires that 'this' be a Number")
		}

		numVal := thisNum.ToFloat()

		// Handle special cases first
		if math.IsNaN(numVal) {
			return vm.NewString("NaN"), nil
		}
		if math.IsInf(numVal, 1) {
			return vm.NewString("Infinity"), nil
		}
		if math.IsInf(numVal, -1) {
			return vm.NewString("-Infinity"), nil
		}

		// Normalize -0 to 0 (per ECMAScript spec, toExponential(-0) returns "0e+0")
		if numVal == 0 {
			numVal = math.Abs(numVal) // This converts -0 to +0
		}

		// Check fractionDigits argument
		digits := -1 // -1 means "as many as needed"
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			// ToInteger the fractionDigits
			fd := args[0].ToFloat()
			if math.IsNaN(fd) {
				fd = 0
			}
			digits = int(fd)
			if digits < 0 || digits > 100 {
				return vm.Undefined, vmInstance.NewRangeError("toExponential() digits argument must be between 0 and 100")
			}
		}

		var result string
		if digits == -1 {
			// Use minimum digits needed (Go's default precision)
			result = strconv.FormatFloat(numVal, 'e', -1, 64)
		} else {
			result = strconv.FormatFloat(numVal, 'e', digits, 64)
		}

		// Remove leading zeros from exponent (e.g., "1e+02" -> "1e+2")
		return vm.NewString(formatExponent(result)), nil
	}))

	numberProto.SetOwnNonEnumerable("toPrecision", vm.NewNativeFunction(1, false, "toPrecision", func(args []vm.Value) (vm.Value, error) {
		thisNum := vmInstance.GetThis()

		// Extract primitive value from wrapper if needed
		if thisNum.IsObject() {
			if primitiveVal, exists := thisNum.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				thisNum = primitiveVal
			}
		}

		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber {
			return vm.Undefined, vmInstance.NewTypeError("Number.prototype.toPrecision requires that 'this' be a Number")
		}

		numVal := thisNum.ToFloat()

		// If precision is undefined, return ToString(this)
		if len(args) == 0 || args[0].Type() == vm.TypeUndefined {
			// Handle special cases
			if math.IsNaN(numVal) {
				return vm.NewString("NaN"), nil
			}
			if math.IsInf(numVal, 1) {
				return vm.NewString("Infinity"), nil
			}
			if math.IsInf(numVal, -1) {
				return vm.NewString("-Infinity"), nil
			}
			return vm.NewString(thisNum.ToString()), nil
		}

		// ToInteger the precision
		p := args[0].ToFloat()
		if math.IsNaN(p) {
			p = 0
		}
		precision := int(p)

		// Per ECMAScript spec: Check NaN/Infinity BEFORE range error
		// See https://tc39.es/ecma262/#sec-number.prototype.toprecision steps 4-7
		if math.IsNaN(numVal) {
			return vm.NewString("NaN"), nil
		}
		if math.IsInf(numVal, 1) {
			return vm.NewString("Infinity"), nil
		}
		if math.IsInf(numVal, -1) {
			return vm.NewString("-Infinity"), nil
		}

		// Now check range (after NaN/Infinity checks)
		if precision < 1 || precision > 100 {
			return vm.Undefined, vmInstance.NewRangeError("toPrecision() argument must be between 1 and 100")
		}

		// Handle -0 as 0 (per ECMAScript spec)
		if numVal == 0 {
			numVal = math.Abs(numVal) // Normalize -0 to +0
		}

		// Use custom precision formatting to match JS behavior
		result := formatToPrecision(numVal, precision)

		return vm.NewString(result), nil
	}))

	// Set Number.prototype
	vmInstance.NumberPrototype = vm.NewValueFromPlainObject(numberProto)

	// Create Number constructor function
	numberConstructor := vm.NewConstructorWithProps(1, false, "Number", func(args []vm.Value) (vm.Value, error) {
		// Determine the primitive number value
		var primitiveValue float64
		if len(args) == 0 {
			primitiveValue = 0
		} else {
			arg := args[0]

			// Handle boxed primitives by extracting primitive value
			if arg.IsObject() {
				obj := arg.AsPlainObject()
				if primVal, found := obj.GetOwn("[[PrimitiveValue]]"); found && primVal != vm.Undefined {
					arg = primVal
				}
			}

			switch arg.Type() {
			case vm.TypeFloatNumber, vm.TypeIntegerNumber:
				primitiveValue = arg.ToFloat()
			case vm.TypeString:
				str := arg.ToString()
				// Trim whitespace (ECMAScript whitespace)
				str = strings.TrimSpace(str)
				str = strings.Trim(str, "\u00A0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200A\u2028\u2029\u202F\u205F\u3000\uFEFF")

				if str == "" {
					primitiveValue = 0
				} else if str == "Infinity" || str == "+Infinity" {
					// ECMAScript only accepts exact case "Infinity"
					primitiveValue = math.Inf(1)
				} else if str == "-Infinity" {
					primitiveValue = math.Inf(-1)
				} else if strings.EqualFold(str, "infinity") || strings.EqualFold(str, "+infinity") || strings.EqualFold(str, "-infinity") ||
					strings.EqualFold(str, "inf") || strings.EqualFold(str, "+inf") || strings.EqualFold(str, "-inf") {
					// Reject case-insensitive infinity variants that aren't exactly "Infinity"/"+Infinity"/"-Infinity"
					primitiveValue = math.NaN()
				} else if len(str) > 2 && (str[0:2] == "0x" || str[0:2] == "0X") {
					// Parse hex string (0x or 0X prefix)
					if val, err := strconv.ParseInt(str[2:], 16, 64); err == nil {
						primitiveValue = float64(val)
					} else {
						primitiveValue = math.NaN()
					}
				} else if len(str) > 2 && (str[0:2] == "0b" || str[0:2] == "0B") {
					// Parse binary string (0b or 0B prefix)
					if val, err := strconv.ParseInt(str[2:], 2, 64); err == nil {
						primitiveValue = float64(val)
					} else {
						primitiveValue = math.NaN()
					}
				} else if len(str) > 2 && (str[0:2] == "0o" || str[0:2] == "0O") {
					// Parse octal string (0o or 0O prefix)
					if val, err := strconv.ParseInt(str[2:], 8, 64); err == nil {
						primitiveValue = float64(val)
					} else {
						primitiveValue = math.NaN()
					}
				} else if strings.Contains(str, "_") {
					// Numeric separators are not allowed in string parsing (only in literal syntax)
					primitiveValue = math.NaN()
				} else if val, err := strconv.ParseFloat(str, 64); err == nil {
					primitiveValue = val
				} else if math.IsInf(val, 0) {
					// Overflow to infinity should be returned, not treated as error
					primitiveValue = val
				} else {
					primitiveValue = math.NaN()
				}
			case vm.TypeBoolean:
				if arg.AsBoolean() {
					primitiveValue = 1
				} else {
					primitiveValue = 0
				}
			case vm.TypeBigInt:
				// BigInt to Number conversion
				primitiveValue = arg.ToFloat()
			case vm.TypeNull:
				primitiveValue = 0
			case vm.TypeUndefined:
				primitiveValue = math.NaN()
			default:
				primitiveValue = math.NaN()
			}
		}

		// If called with 'new', return a Number wrapper object
		if vmInstance.IsConstructorCall() {
			return vmInstance.NewNumberObject(primitiveValue), nil
		}
		// Otherwise, return primitive number (type coercion)
		return vm.NumberValue(primitiveValue), nil
	})

	// Add Number static properties (non-writable, non-enumerable, non-configurable per ECMAScript spec)
	writable := false
	enumerable := false
	configurable := false
	numberConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("MAX_VALUE", vm.NumberValue(math.MaxFloat64), &writable, &enumerable, &configurable)
	numberConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("MIN_VALUE", vm.NumberValue(math.SmallestNonzeroFloat64), &writable, &enumerable, &configurable)
	numberConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("NaN", vm.NaN, &writable, &enumerable, &configurable)
	numberConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("NEGATIVE_INFINITY", vm.NumberValue(math.Inf(-1)), &writable, &enumerable, &configurable)
	numberConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("POSITIVE_INFINITY", vm.NumberValue(math.Inf(1)), &writable, &enumerable, &configurable)
	numberConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("MAX_SAFE_INTEGER", vm.NumberValue(9007199254740991), &writable, &enumerable, &configurable)  // 2^53 - 1
	numberConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("MIN_SAFE_INTEGER", vm.NumberValue(-9007199254740991), &writable, &enumerable, &configurable) // -(2^53 - 1)
	numberConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("EPSILON", vm.NumberValue(math.Nextafter(1.0, 2.0)-1.0), &writable, &enumerable, &configurable)

	// Add Number static methods
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("isNaN", vm.NewNativeFunction(1, false, "isNaN", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}
		val := args[0]
		if val.Type() != vm.TypeFloatNumber && val.Type() != vm.TypeIntegerNumber {
			return vm.BooleanValue(false), nil // Only numbers can be NaN
		}
		return vm.BooleanValue(math.IsNaN(val.ToFloat())), nil
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("isFinite", vm.NewNativeFunction(1, false, "isFinite", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}
		val := args[0]
		if val.Type() != vm.TypeFloatNumber && val.Type() != vm.TypeIntegerNumber {
			return vm.BooleanValue(false), nil // Only numbers can be finite
		}
		f := val.ToFloat()
		return vm.BooleanValue(!math.IsInf(f, 0) && !math.IsNaN(f)), nil
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("isInteger", vm.NewNativeFunction(1, false, "isInteger", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}
		val := args[0]
		if val.Type() != vm.TypeFloatNumber && val.Type() != vm.TypeIntegerNumber {
			return vm.BooleanValue(false), nil
		}
		f := val.ToFloat()
		return vm.BooleanValue(!math.IsInf(f, 0) && !math.IsNaN(f) && math.Floor(f) == f), nil
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("isSafeInteger", vm.NewNativeFunction(1, false, "isSafeInteger", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}
		val := args[0]
		if val.Type() != vm.TypeFloatNumber && val.Type() != vm.TypeIntegerNumber {
			return vm.BooleanValue(false), nil
		}
		f := val.ToFloat()
		maxSafe := 9007199254740991.0 // 2^53 - 1
		return vm.BooleanValue(!math.IsInf(f, 0) && !math.IsNaN(f) && math.Floor(f) == f && f >= -maxSafe && f <= maxSafe), nil
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("parseFloat", vm.NewNativeFunction(1, false, "parseFloat", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NaN, nil
		}
		str := args[0].ToString()
		if val, err := strconv.ParseFloat(str, 64); err == nil {
			return vm.NumberValue(val), nil
		}
		return vm.NaN, nil
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("parseInt", vm.NewNativeFunction(2, false, "parseInt", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NaN, nil
		}

		str := args[0].ToString()
		radix := 10
		if len(args) > 1 {
			radix = int(args[1].ToFloat())
		}

		if radix == 0 {
			radix = 10 // Default radix
		}
		if radix < 2 || radix > 36 {
			return vm.NaN, nil
		}

		if val, err := strconv.ParseInt(str, radix, 64); err == nil {
			return vm.NumberValue(float64(val)), nil
		}
		return vm.NaN, nil
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.NumberPrototype)

	// Set constructor property on prototype
	numberProto.SetOwnNonEnumerable("constructor", numberConstructor)

	// Define Number constructor in global scope
	return ctx.DefineGlobal("Number", numberConstructor)
}

package builtins

import (
	"math/big"
	"strings"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type BigIntInitializer struct{}

func (b *BigIntInitializer) Name() string {
	return "BigInt"
}

func (b *BigIntInitializer) Priority() int {
	return 360 // After Number (350)
}

func (b *BigIntInitializer) InitTypes(ctx *TypeContext) error {
	// Create BigInt constructor type
	bigintCtorType := types.NewSimpleFunction([]types.Type{types.Any}, types.BigInt).
		WithProperty("asIntN", types.NewSimpleFunction([]types.Type{types.Number, types.BigInt}, types.BigInt)).
		WithProperty("asUintN", types.NewSimpleFunction([]types.Type{types.Number, types.BigInt}, types.BigInt))

	// Create BigInt.prototype type with all methods
	// Note: 'this' is implicit and not included in type signatures
	bigintProtoType := types.NewObjectType().
		WithProperty("toString", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("toLocaleString", types.NewOptionalFunction([]types.Type{types.String, types.Any}, types.String, []bool{true, true})).
		WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.BigInt)).
		WithProperty("constructor", types.Any) // Avoid circular reference, use Any for constructor property

	// Register BigInt primitive prototype
	ctx.SetPrimitivePrototype("bigint", bigintProtoType)

	// Add prototype property to constructor
	bigintCtorType = bigintCtorType.WithProperty("prototype", bigintProtoType)

	// Define BigInt constructor in global environment
	return ctx.DefineGlobal("BigInt", bigintCtorType)
}

// thisBigIntValue implements the spec's ThisBigIntValue: a primitive BigInt is
// its own value, and a BigInt object wrapper yields its wrapped primitive.
// Any other receiver reports false.
//
// The slot is spelled "[[PrimitiveValue]]" because that is what actually
// creates BigInt wrappers here (object_init.go's Object(bigint) case); the
// spec's name for it is [[BigIntData]]. Number/String/Boolean/Symbol wrappers
// share the same slot name, so the TypeBigInt check on the stored value is
// what keeps this from unwrapping one of those.
//
// This lives in one place on purpose. Each call site previously inlined the
// unwrap guarded by `Type() == TypeBigInt` and then called AsPlainObject() on
// it — but AsPlainObject panics unless the value is TypeObject, and a wrapper
// is TypeObject, never TypeBigInt. So the wrapper branch was unreachable and
// every primitive receiver panicked.
func thisBigIntValue(v vm.Value) (vm.Value, bool) {
	switch v.Type() {
	case vm.TypeBigInt:
		return v, true
	case vm.TypeObject:
		if v.Type() == vm.TypeObject {
			po := v.AsPlainObject()
			if data, exists := po.GetOwn("[[PrimitiveValue]]"); exists && data.Type() == vm.TypeBigInt {
				return data, true
			}
		}
	}
	return vm.Undefined, false
}

func (b *BigIntInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create BigInt.prototype inheriting from Object.prototype
	bigintProto := vm.NewObject(objectProto).AsPlainObject()

	// Add BigInt prototype methods
	bigintProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(1, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisBigInt := vmInstance.GetThis()

		// Get the primitive BigInt value
		primitiveBigInt, ok := thisBigIntValue(thisBigInt)
		if !ok {
			// For non-BigInts, try to convert or throw error
			return vm.NewString(thisBigInt.ToString()), nil
		}

		// Radix per BigInt.prototype.toString: undefined means 10, otherwise
		// ToIntegerOrInfinity (which throws on a Symbol or a BigInt-returning
		// ToPrimitive), then RangeError outside 2..36.
		var radix int = 10
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			r, err := toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
			if r < 2 || r > 36 {
				return vm.Undefined, vmInstance.NewRangeError("toString() radix must be between 2 and 36")
			}
			radix = r
		}

		bigIntVal := primitiveBigInt.AsBigInt()
		if radix == 10 {
			return vm.NewString(bigIntVal.String()), nil
		}

		// Handle different radix
		return vm.NewString(bigIntVal.Text(radix)), nil
	}))

	bigintProto.SetOwnNonEnumerable("toLocaleString", vm.NewNativeFunction(2, false, "toLocaleString", func(args []vm.Value) (vm.Value, error) {
		thisBigInt := vmInstance.GetThis()

		// Get the primitive BigInt value
		primitiveBigInt, ok := thisBigIntValue(thisBigInt)
		if !ok {
			// For non-BigInts, try to convert or throw error
			return vm.NewString(thisBigInt.ToString()), nil
		}

		// For now, just return the string representation (proper locale support would be complex)
		// TODO: Implement proper locale formatting
		return vm.NewString(primitiveBigInt.AsBigInt().String()), nil
	}))

	bigintProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		thisBigInt := vmInstance.GetThis()

		// Return the primitive BigInt value
		if primitiveBigInt, ok := thisBigIntValue(thisBigInt); ok {
			return primitiveBigInt, nil
		}

		// Cannot convert other types to BigInt
		return vm.Undefined, vmInstance.NewTypeError("Cannot convert to BigInt")
	}))

	// Add BigInt.prototype[@@toStringTag] = "BigInt" (writable: false, enumerable: false, configurable: true)
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		wFalse, eFalse, cTrue := false, false, true
		bigintProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("BigInt"),
			&wFalse, &eFalse, &cTrue,
		)
	}

	// Set BigInt.prototype
	vmInstance.BigIntPrototype = vm.NewValueFromPlainObject(bigintProto)

	// Create BigInt constructor function
	bigintConstructor := vm.NewNativeFunctionWithProps(1, false, "BigInt", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			// BigInt() without arguments should throw TypeError
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined to a BigInt")
		}

		arg := args[0]

		// If argument is already a BigInt, return its primitive value. This
		// covers both a primitive and an object wrapper, per ToBigInt.
		if primitive, ok := thisBigIntValue(arg); ok {
			return primitive, nil
		}

		// Convert argument to primitive BigInt
		switch arg.Type() {
		case vm.TypeString:
			str := strings.TrimSpace(arg.ToString())
			if str == "" {
				// Empty string should throw SyntaxError
				return vm.Undefined, vmInstance.NewSyntaxError("Cannot convert empty string to BigInt")
			}

			// Try to parse as BigInt
			bigVal := new(big.Int)
			if _, ok := bigVal.SetString(str, 0); !ok {
				return vm.Undefined, vmInstance.NewSyntaxError("Cannot convert string to BigInt")
			}
			return vm.NewBigInt(bigVal), nil
		case vm.TypeIntegerNumber:
			// Convert integer to BigInt
			intVal := arg.AsInteger()
			bigVal := big.NewInt(int64(intVal))
			return vm.NewBigInt(bigVal), nil
		case vm.TypeFloatNumber:
			// Check if float is actually an integer
			floatVal := arg.ToFloat()
			if floatVal != float64(int64(floatVal)) {
				return vm.Undefined, vmInstance.NewRangeError("Cannot convert non-integer number to BigInt")
			}
			bigVal := big.NewInt(int64(floatVal))
			return vm.NewBigInt(bigVal), nil
		case vm.TypeBoolean:
			if arg.AsBoolean() {
				return vm.NewBigInt(big.NewInt(1)), nil
			}
			return vm.NewBigInt(big.NewInt(0)), nil
		case vm.TypeNull, vm.TypeUndefined:
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert null/undefined to BigInt")
		default:
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert to BigInt")
		}
	})

	// Add BigInt static methods
	bigintConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("asIntN", vm.NewNativeFunction(2, false, "asIntN", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("BigInt.asIntN requires 2 arguments")
		}

		bits := int(args[0].ToFloat())
		bigintVal := args[1]

		if bigintVal.Type() != vm.TypeBigInt {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert to BigInt")
		}

		if bits < 0 {
			return vm.Undefined, vmInstance.NewRangeError("Invalid bit width")
		}

		// Truncate to N bits with sign extension
		val := bigintVal.AsBigInt()
		result := new(big.Int).Set(val)

		// For now, just return the original value (proper implementation would require bit manipulation)
		// TODO: Implement proper N-bit signed integer truncation
		return vm.NewBigInt(result), nil
	}))

	bigintConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("asUintN", vm.NewNativeFunction(2, false, "asUintN", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("BigInt.asUintN requires 2 arguments")
		}

		bits := int(args[0].ToFloat())
		bigintVal := args[1]

		if bigintVal.Type() != vm.TypeBigInt {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert to BigInt")
		}

		if bits < 0 {
			return vm.Undefined, vmInstance.NewRangeError("Invalid bit width")
		}

		// Truncate to N bits without sign extension
		val := bigintVal.AsBigInt()
		result := new(big.Int).Set(val)

		// For now, just return the original value (proper implementation would require bit manipulation)
		// TODO: Implement proper N-bit unsigned integer truncation
		return vm.NewBigInt(result), nil
	}))

	bigintConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.BigIntPrototype)

	// Set constructor property on prototype
	bigintProto.SetOwnNonEnumerable("constructor", bigintConstructor)

	// Define BigInt constructor in global scope
	return ctx.DefineGlobal("BigInt", bigintConstructor)
}

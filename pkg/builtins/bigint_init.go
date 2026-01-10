package builtins

import (
	"fmt"
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
		var primitiveBigInt vm.Value
		if thisBigInt.Type() == vm.TypeBigInt {
			// For BigInt object wrappers, extract the primitive value from [[BigIntData]]
			if po := thisBigInt.AsPlainObject(); po != nil {
				if dataVal, exists := po.GetOwn("[[BigIntData]]"); exists {
					primitiveBigInt = dataVal
				} else {
					primitiveBigInt = thisBigInt
				}
			} else {
				primitiveBigInt = thisBigInt
			}
		} else {
			// For non-BigInts, try to convert or throw error
			return vm.NewString(thisBigInt.ToString()), nil
		}

		var radix int = 10
		if len(args) > 0 {
			radix = int(args[0].ToFloat())
			if radix < 2 || radix > 36 {
				// In real JS this would throw RangeError, for now use default
				radix = 10
			}
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
		var primitiveBigInt vm.Value
		if thisBigInt.Type() == vm.TypeBigInt {
			// For BigInt object wrappers, extract the primitive value from [[BigIntData]]
			if po := thisBigInt.AsPlainObject(); po != nil {
				if dataVal, exists := po.GetOwn("[[BigIntData]]"); exists {
					primitiveBigInt = dataVal
				} else {
					primitiveBigInt = thisBigInt
				}
			} else {
				primitiveBigInt = thisBigInt
			}
		} else {
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
		if thisBigInt.Type() == vm.TypeBigInt {
			// For BigInt object wrappers, extract the primitive value from [[BigIntData]]
			if po := thisBigInt.AsPlainObject(); po != nil {
				if dataVal, exists := po.GetOwn("[[BigIntData]]"); exists {
					return dataVal, nil
				}
			}
			return thisBigInt, nil
		}

		// Cannot convert other types to BigInt
		// In real JS this would throw TypeError
		return vm.Undefined, fmt.Errorf("TypeError: Cannot convert to BigInt")
	}))

	// Set BigInt.prototype
	vmInstance.BigIntPrototype = vm.NewValueFromPlainObject(bigintProto)

	// Create BigInt constructor function
	bigintConstructor := vm.NewNativeFunctionWithProps(1, false, "BigInt", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			// BigInt() without arguments should throw TypeError
			return vm.Undefined, fmt.Errorf("TypeError: BigInt constructor requires an argument")
		}

		arg := args[0]

		// If argument is already a BigInt, return it (handles both primitive and object wrapper cases)
		if arg.Type() == vm.TypeBigInt {
			// Check if it's already an object wrapper
			if po := arg.AsPlainObject(); po != nil {
				if _, exists := po.GetOwn("[[BigIntData]]"); exists {
					// Already an object wrapper, return as-is
					return arg, nil
				}
			}
			// It's a primitive BigInt, return as-is
			return arg, nil
		}

		// Convert argument to primitive BigInt
		switch arg.Type() {
		case vm.TypeString:
			str := strings.TrimSpace(arg.ToString())
			if str == "" {
				// Empty string should throw SyntaxError
				return vm.Undefined, fmt.Errorf("SyntaxError: Cannot convert empty string to BigInt")
			}

			// Try to parse as BigInt
			bigVal := new(big.Int)
			if _, ok := bigVal.SetString(str, 0); !ok {
				return vm.Undefined, fmt.Errorf("SyntaxError: Cannot convert string to BigInt")
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
				return vm.Undefined, fmt.Errorf("RangeError: Cannot convert non-integer number to BigInt")
			}
			bigVal := big.NewInt(int64(floatVal))
			return vm.NewBigInt(bigVal), nil
		case vm.TypeBoolean:
			if arg.AsBoolean() {
				return vm.NewBigInt(big.NewInt(1)), nil
			}
			return vm.NewBigInt(big.NewInt(0)), nil
		case vm.TypeNull, vm.TypeUndefined:
			return vm.Undefined, fmt.Errorf("TypeError: Cannot convert null/undefined to BigInt")
		default:
			return vm.Undefined, fmt.Errorf("TypeError: Cannot convert to BigInt")
		}
	})

	// Add BigInt static methods
	bigintConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("asIntN", vm.NewNativeFunction(2, false, "asIntN", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, fmt.Errorf("TypeError: BigInt.asIntN requires 2 arguments")
		}

		bits := int(args[0].ToFloat())
		bigintVal := args[1]

		if bigintVal.Type() != vm.TypeBigInt {
			return vm.Undefined, fmt.Errorf("TypeError: Second argument must be a BigInt")
		}

		if bits < 0 || bits > 64 {
			return vm.Undefined, fmt.Errorf("RangeError: Invalid bit width")
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
			return vm.Undefined, fmt.Errorf("TypeError: BigInt.asUintN requires 2 arguments")
		}

		bits := int(args[0].ToFloat())
		bigintVal := args[1]

		if bigintVal.Type() != vm.TypeBigInt {
			return vm.Undefined, fmt.Errorf("TypeError: Second argument must be a BigInt")
		}

		if bits < 0 || bits > 64 {
			return vm.Undefined, fmt.Errorf("RangeError: Invalid bit width")
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

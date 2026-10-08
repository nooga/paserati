package builtins

import (
	"math"
	"math/big"

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
	bigintCtorType := types.NewSimpleFunction([]types.Type{types.NewUnionType(types.BigInt, types.Boolean, types.Number, types.String)}, types.BigInt).
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
			if data, exists := po.GetInternal("[[PrimitiveValue]]"); exists && data.Type() == vm.TypeBigInt {
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

	bigintProto.SetOwnNonEnumerable("toLocaleString", vm.NewNativeFunction(0, false, "toLocaleString", func(args []vm.Value) (vm.Value, error) {
		primitiveBigInt, ok := thisBigIntValue(vmInstance.GetThis())
		if !ok {
			return vm.Undefined, vmInstance.NewTypeError("BigInt.prototype.toLocaleString requires that 'this' be a BigInt")
		}
		return intlFormatNumberWith(vmInstance, primitiveBigInt, intlArg(args, 0), intlArg(args, 1))
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
			bigVal, ok := vm.StringToBigInt(arg.ToString())
			if !ok {
				return vm.Undefined, vmInstance.NewSyntaxError("Cannot convert " + arg.ToString() + " to a BigInt")
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
	// BigInt.asIntN / asUintN (21.2.2.1-2): wrap to the low `bits` bits,
	// as a two's-complement signed value or as an unsigned one.
	asN := func(name string, signed bool) vm.Value {
		return vm.NewNativeFunction(2, false, name, func(args []vm.Value) (vm.Value, error) {
			var bitsArg, bigintArg vm.Value = vm.Undefined, vm.Undefined
			if len(args) > 0 {
				bitsArg = args[0]
			}
			if len(args) > 1 {
				bigintArg = args[1]
			}
			bits, err := toIndexWithVM(vmInstance, bitsArg)
			if err != nil {
				return vm.Undefined, err
			}
			n, err := toBigIntWithVM(vmInstance, bigintArg)
			if err != nil {
				return vm.Undefined, err
			}
			return vm.NewBigInt(wrapBigIntBits(n, bits, signed)), nil
		})
	}
	bigintConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("asIntN", asN("asIntN", true))
	bigintConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("asUintN", asN("asUintN", false))

	bigintConstructor.AsNativeFunctionWithProps().Properties.DefineFixedProperty("prototype", vmInstance.BigIntPrototype)

	// Set constructor property on prototype
	bigintProto.SetOwnNonEnumerable("constructor", bigintConstructor)

	// Define BigInt constructor in global scope
	return ctx.DefineGlobal("BigInt", bigintConstructor)
}

// wrapBigIntBits returns n modulo 2^bits, read as a signed (two's
// complement) or unsigned bits-wide integer.
func wrapBigIntBits(n *big.Int, bits int, signed bool) *big.Int {
	if bits == 0 {
		return big.NewInt(0)
	}
	// Past this width every BigInt we can hold is already in range; skip
	// building 2^bits for a huge bits.
	if n.BitLen() < bits {
		if !signed && n.Sign() < 0 {
			// Negative values still wrap: n + 2^bits.
			mod := new(big.Int).Lsh(big.NewInt(1), uint(bits))
			return mod.Add(mod, n)
		}
		return new(big.Int).Set(n)
	}
	mod := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	r := new(big.Int).Mod(n, mod) // Euclidean: 0 <= r < 2^bits
	if signed && r.Bit(bits-1) == 1 {
		r.Sub(r, mod)
	}
	return r
}

// toIndexWithVM is ToIndex (7.1.22): ToIntegerOrInfinity, then a RangeError
// outside [0, 2^53-1].
func toIndexWithVM(vmInstance *vm.VM, v vm.Value) (int, error) {
	if v.Type() == vm.TypeUndefined {
		return 0, nil
	}
	if v.Type() == vm.TypeSymbol {
		return 0, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
	}
	if v.Type() == vm.TypeBigInt {
		return 0, vmInstance.NewTypeError("Cannot convert a BigInt value to a number")
	}
	vmInstance.EnterHelperCall()
	f := vmInstance.ToNumber(v)
	vmInstance.ExitHelperCall()
	if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
		return 0, ErrVMUnwinding
	}
	if math.IsNaN(f) {
		f = 0
	}
	f = math.Trunc(f)
	if f < 0 || f > maxSafeInteger {
		return 0, vmInstance.NewRangeError("Invalid value: not (convertible to) a safe integer")
	}
	return int(f), nil
}

// toBigIntWithVM is ToBigInt (7.1.13): ToPrimitive(number), then BigInt
// and boolean pass, strings parse with StringToBigInt (SyntaxError if they
// don't), and everything else is a TypeError.
func toBigIntWithVM(vmInstance *vm.VM, v vm.Value) (*big.Int, error) {
	if v.IsObject() || v.IsCallable() {
		vmInstance.EnterHelperCall()
		v = vmInstance.ToPrimitive(v, "number")
		vmInstance.ExitHelperCall()
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return nil, ErrVMUnwinding
		}
	}
	switch v.Type() {
	case vm.TypeBigInt:
		return v.AsBigInt(), nil
	case vm.TypeBoolean:
		if v.AsBoolean() {
			return big.NewInt(1), nil
		}
		return big.NewInt(0), nil
	case vm.TypeString:
		n, ok := vm.StringToBigInt(v.ToString())
		if !ok {
			return nil, vmInstance.NewSyntaxError("Cannot convert " + v.ToString() + " to a BigInt")
		}
		return n, nil
	case vm.TypeUndefined:
		return nil, vmInstance.NewTypeError("Cannot convert undefined to a BigInt")
	case vm.TypeNull:
		return nil, vmInstance.NewTypeError("Cannot convert null to a BigInt")
	case vm.TypeSymbol:
		return nil, vmInstance.NewTypeError("Cannot convert a Symbol value to a BigInt")
	}
	return nil, vmInstance.NewTypeError("Cannot convert " + v.ToString() + " to a BigInt")
}

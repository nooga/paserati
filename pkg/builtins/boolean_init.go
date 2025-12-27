package builtins

import (
	"fmt"
	"math"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type BooleanInitializer struct{}

func (b *BooleanInitializer) Name() string {
	return "Boolean"
}

func (b *BooleanInitializer) Priority() int {
	return 340 // After Number (350), before String (300)
}

func (b *BooleanInitializer) InitTypes(ctx *TypeContext) error {
	// Create Boolean constructor type first (needed for constructor property)
	booleanCtorType := types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)

	// Create Boolean.prototype type with all methods
	// Note: 'this' is implicit and not included in type signatures
	booleanProtoType := types.NewObjectType().
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.Boolean)).
		WithProperty("constructor", types.Any) // Avoid circular reference, use Any for constructor property

	// Register boolean primitive prototype
	ctx.SetPrimitivePrototype("boolean", booleanProtoType)

	// Add prototype property to constructor
	booleanCtorType = booleanCtorType.WithProperty("prototype", booleanProtoType)

	// Define Boolean constructor in global environment
	return ctx.DefineGlobal("Boolean", booleanCtorType)
}

func (b *BooleanInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Boolean.prototype inheriting from Object.prototype
	booleanProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Boolean prototype methods
	booleanProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisBool := vmInstance.GetThis()

		// If this is a primitive boolean, convert it
		if thisBool.Type() == vm.TypeBoolean {
			boolVal := thisBool.AsBoolean()
			if boolVal {
				return vm.NewString("true"), nil
			}
			return vm.NewString("false"), nil
		}

		// If this is a Boolean wrapper object, extract [[PrimitiveValue]]
		if thisBool.IsObject() {
			if primitiveVal, exists := thisBool.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				if primitiveVal.Type() == vm.TypeBoolean {
					boolVal := primitiveVal.AsBoolean()
					if boolVal {
						return vm.NewString("true"), nil
					}
					return vm.NewString("false"), nil
				}
			}
		}

		// TypeError: Boolean.prototype.toString requires that 'this' be a Boolean
		return vm.Undefined, fmt.Errorf("Boolean.prototype.toString requires that 'this' be a Boolean")
	}))

	booleanProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		thisBool := vmInstance.GetThis()

		// If this is a primitive boolean, return it
		if thisBool.Type() == vm.TypeBoolean {
			return thisBool, nil
		}

		// If this is a Boolean wrapper object, extract [[PrimitiveValue]]
		if thisBool.IsObject() {
			if primitiveVal, exists := thisBool.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				return primitiveVal, nil
			}
		}

		// TypeError: Boolean.prototype.valueOf requires that 'this' be a Boolean
		return vm.Undefined, fmt.Errorf("Boolean.prototype.valueOf requires that 'this' be a Boolean")
	}))

	// Set Boolean.prototype
	vmInstance.BooleanPrototype = vm.NewValueFromPlainObject(booleanProto)

	// Create Boolean constructor function
	booleanConstructor := vm.NewConstructorWithProps(1, false, "Boolean", func(args []vm.Value) (vm.Value, error) {
		// Determine the primitive boolean value
		var primitiveValue bool
		if len(args) == 0 {
			primitiveValue = false
		} else {
			arg := args[0]
			switch arg.Type() {
			case vm.TypeBoolean:
				primitiveValue = arg.AsBoolean()
			case vm.TypeString:
				str := arg.ToString()
				primitiveValue = str != "" && str != "0" && str != "false"
			case vm.TypeFloatNumber, vm.TypeIntegerNumber:
				num := arg.ToFloat()
				primitiveValue = num != 0 && !math.IsNaN(num)
			case vm.TypeNull:
				primitiveValue = false
			case vm.TypeUndefined:
				primitiveValue = false
			default:
				// For objects, try valueOf/toString
				if arg.IsObject() {
					// Try valueOf first
					if valueOf, exists := arg.AsPlainObject().GetOwn("valueOf"); exists && valueOf.IsFunction() {
						result, err := vmInstance.Call(valueOf, arg, nil)
						if err == nil && !result.IsObject() {
							primitiveValue = result.IsTruthy()
						} else {
							primitiveValue = arg.IsTruthy()
						}
					} else if toString, exists := arg.AsPlainObject().GetOwn("toString"); exists && toString.IsFunction() {
						// Try toString
						result, err := vmInstance.Call(toString, arg, nil)
						if err == nil && !result.IsObject() {
							primitiveValue = result.IsTruthy()
						} else {
							primitiveValue = arg.IsTruthy()
						}
					} else {
						primitiveValue = arg.IsTruthy()
					}
				} else {
					primitiveValue = arg.IsTruthy()
				}
			}
		}

		// If called with 'new', return a Boolean wrapper object
		if vmInstance.IsConstructorCall() {
			return vmInstance.NewBooleanObject(primitiveValue), nil
		}
		// Otherwise, return primitive boolean (type coercion)
		return vm.BooleanValue(primitiveValue), nil
	})

	// Add prototype property to constructor
	booleanConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.BooleanPrototype)

	// Set constructor property on prototype
	booleanProto.SetOwnNonEnumerable("constructor", booleanConstructor)

	// Define Boolean constructor in global scope
	return ctx.DefineGlobal("Boolean", booleanConstructor)
}

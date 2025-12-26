package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// TypeErrorInitializer implements the TypeError constructor and TypeError.prototype
type TypeErrorInitializer struct{}

func (t *TypeErrorInitializer) Name() string {
	return "TypeError"
}

func (t *TypeErrorInitializer) Priority() int {
	return 21 // After Error
}

func (t *TypeErrorInitializer) InitTypes(ctx *TypeContext) error {
	// Create TypeError.prototype type (inherits from Error.prototype)
	typeErrorProtoType := types.NewObjectType().
		WithProperty("name", types.String).
		WithProperty("message", types.String).
		WithProperty("stack", types.String).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String))

	// Create TypeError constructor type
	typeErrorCtorType := types.NewObjectType().
		// Constructor is callable with optional message parameter
		WithSimpleCallSignature([]types.Type{}, typeErrorProtoType).
		WithSimpleCallSignature([]types.Type{types.String}, typeErrorProtoType).
		WithProperty("prototype", typeErrorProtoType)

	// Define the constructor globally
	return ctx.DefineGlobal("TypeError", typeErrorCtorType)
}

func (t *TypeErrorInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Error.prototype to inherit from
	errorPrototype := vmInstance.ErrorPrototype
	if errorPrototype.Type() == vm.TypeUndefined {
		// Error hasn't been initialized yet, this shouldn't happen with proper priority
		return nil
	}

	// Create TypeError.prototype object that inherits from Error.prototype
	typeErrorPrototype := vm.NewObject(errorPrototype).AsPlainObject()
	
	// Override the name property to "TypeError"
	typeErrorPrototype.SetOwnNonEnumerable("name", vm.NewString("TypeError"))
	
	// TypeError constructor function
	typeErrorConstructor := vm.NewNativeFunction(-1, true, "TypeError", func(args []vm.Value) (vm.Value, error) {
		// Get message argument
		var message string
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			message = args[0].ToString()
		}
		
		// Create new TypeError instance that inherits from TypeError.prototype
		typeErrorInstance := vm.NewObject(vm.NewValueFromPlainObject(typeErrorPrototype))
		typeErrorInstancePtr := typeErrorInstance.AsPlainObject()
		
		// Set properties (override name, set message and stack)
		typeErrorInstancePtr.SetOwnNonEnumerable("name", vm.NewString("TypeError"))
		typeErrorInstancePtr.SetOwnNonEnumerable("message", vm.NewString(message))
		
		// Capture stack trace at the time of TypeError creation
		stackTrace := vmInstance.CaptureStackTrace()
		typeErrorInstancePtr.SetOwnNonEnumerable("stack", vm.NewString(stackTrace))
		
		return typeErrorInstance, nil
	})

	// Make it a proper constructor with prototype property
	if ctorObj := typeErrorConstructor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties
		ctorWithProps := vm.NewNativeFunctionWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(typeErrorPrototype))

		typeErrorConstructor = ctorWithProps
	}

	// Set constructor property on prototype
	typeErrorPrototype.SetOwnNonEnumerable("constructor", typeErrorConstructor)

	// Store in VM for later use
	vmInstance.TypeErrorPrototype = vm.NewValueFromPlainObject(typeErrorPrototype)

	// Define globally
	return ctx.DefineGlobal("TypeError", typeErrorConstructor)
}

// InitTypeError creates and returns a TypeErrorInitializer
func InitTypeError() BuiltinInitializer {
	return &TypeErrorInitializer{}
}
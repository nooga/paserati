package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// ReferenceErrorInitializer implements the ReferenceError constructor and ReferenceError.prototype
type ReferenceErrorInitializer struct{}

func (r *ReferenceErrorInitializer) Name() string {
	return "ReferenceError"
}

func (r *ReferenceErrorInitializer) Priority() int {
	return 21 // After Error
}

func (r *ReferenceErrorInitializer) InitTypes(ctx *TypeContext) error {
	// Create ReferenceError.prototype type (inherits from Error.prototype)
	referenceErrorProtoType := types.NewObjectType().
		WithProperty("name", types.String).
		WithProperty("message", types.String).
		WithProperty("stack", types.String).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String))

	// Create ReferenceError constructor type
	referenceErrorCtorType := types.NewObjectType().
		// Constructor is callable with optional message parameter
		WithSimpleCallSignature([]types.Type{}, referenceErrorProtoType).
		WithSimpleCallSignature([]types.Type{types.String}, referenceErrorProtoType).
		WithProperty("prototype", referenceErrorProtoType)

	// Define the constructor globally
	return ctx.DefineGlobal("ReferenceError", referenceErrorCtorType)
}

func (r *ReferenceErrorInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Error.prototype to inherit from
	errorPrototype := vmInstance.ErrorPrototype
	if errorPrototype.Type() == vm.TypeUndefined {
		// Error hasn't been initialized yet, this shouldn't happen with proper priority
		return nil
	}

	// Create ReferenceError.prototype object that inherits from Error.prototype
	referenceErrorPrototype := vm.NewObject(errorPrototype).AsPlainObject()
	
	// Override the name property to "ReferenceError"
	referenceErrorPrototype.SetOwnNonEnumerable("name", vm.NewString("ReferenceError"))
	
	// ReferenceError constructor function
	referenceErrorConstructor := vm.NewNativeFunction(-1, true, "ReferenceError", func(args []vm.Value) (vm.Value, error) {
		// Get message argument
		var message string
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			message = args[0].ToString()
		}
		
		// Create new ReferenceError instance that inherits from ReferenceError.prototype
		referenceErrorInstance := vm.NewObject(vm.NewValueFromPlainObject(referenceErrorPrototype))
		referenceErrorInstancePtr := referenceErrorInstance.AsPlainObject()
		
		// Set properties (override name, set message and stack)
		referenceErrorInstancePtr.SetOwnNonEnumerable("name", vm.NewString("ReferenceError"))
		referenceErrorInstancePtr.SetOwnNonEnumerable("message", vm.NewString(message))
		
		// Capture stack trace at the time of ReferenceError creation
		stackTrace := vmInstance.CaptureStackTrace()
		referenceErrorInstancePtr.SetOwnNonEnumerable("stack", vm.NewString(stackTrace))
		
		return referenceErrorInstance, nil
	})

	// Make it a proper constructor with prototype property
	if ctorObj := referenceErrorConstructor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties
		ctorWithProps := vm.NewConstructorWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(referenceErrorPrototype))

		referenceErrorConstructor = ctorWithProps
	}

	// Set constructor property on prototype
	referenceErrorPrototype.SetOwnNonEnumerable("constructor", referenceErrorConstructor)

	// Store in VM for later use
	vmInstance.ReferenceErrorPrototype = vm.NewValueFromPlainObject(referenceErrorPrototype)

	// Define globally
	return ctx.DefineGlobal("ReferenceError", referenceErrorConstructor)
}

// InitReferenceError creates and returns a ReferenceErrorInitializer
func InitReferenceError() BuiltinInitializer {
	return &ReferenceErrorInitializer{}
}
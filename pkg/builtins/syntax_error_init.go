package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// SyntaxErrorInitializer implements the SyntaxError constructor and SyntaxError.prototype
type SyntaxErrorInitializer struct{}

func (s *SyntaxErrorInitializer) Name() string {
	return "SyntaxError"
}

func (s *SyntaxErrorInitializer) Priority() int {
	return 21 // After Error
}

func (s *SyntaxErrorInitializer) InitTypes(ctx *TypeContext) error {
	// Create SyntaxError.prototype type (inherits from Error.prototype)
	syntaxErrorProtoType := types.NewObjectType().
		WithProperty("name", types.String).
		WithProperty("message", types.String).
		WithProperty("stack", types.String).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String))

	// Create SyntaxError constructor type
	syntaxErrorCtorType := types.NewObjectType().
		// Constructor is callable with optional message parameter
		WithSimpleCallSignature([]types.Type{}, syntaxErrorProtoType).
		WithSimpleCallSignature([]types.Type{types.String}, syntaxErrorProtoType).
		WithProperty("prototype", syntaxErrorProtoType)

	// Define the constructor globally
	return ctx.DefineGlobal("SyntaxError", syntaxErrorCtorType)
}

func (s *SyntaxErrorInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Error.prototype to inherit from
	errorPrototype := vmInstance.ErrorPrototype
	if errorPrototype.Type() == vm.TypeUndefined {
		// Error hasn't been initialized yet, this shouldn't happen with proper priority
		return nil
	}

	// Create SyntaxError.prototype object that inherits from Error.prototype
	syntaxErrorPrototype := vm.NewObject(errorPrototype).AsPlainObject()
	
	// Override the name property to "SyntaxError"
	syntaxErrorPrototype.SetOwnNonEnumerable("name", vm.NewString("SyntaxError"))
	
	// SyntaxError constructor function
	syntaxErrorConstructor := vm.NewNativeFunction(-1, true, "SyntaxError", func(args []vm.Value) (vm.Value, error) {
		// Get message argument
		var message string
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			message = args[0].ToString()
		}
		
		// Create new SyntaxError instance that inherits from SyntaxError.prototype
		syntaxErrorInstance := vm.NewObject(vm.NewValueFromPlainObject(syntaxErrorPrototype))
		syntaxErrorInstancePtr := syntaxErrorInstance.AsPlainObject()
		
		// Set properties (override name, set message and stack)
		syntaxErrorInstancePtr.SetOwnNonEnumerable("name", vm.NewString("SyntaxError"))
		syntaxErrorInstancePtr.SetOwnNonEnumerable("message", vm.NewString(message))
		
		// Capture stack trace at the time of SyntaxError creation
		stackTrace := vmInstance.CaptureStackTrace()
		syntaxErrorInstancePtr.SetOwnNonEnumerable("stack", vm.NewString(stackTrace))
		
		return syntaxErrorInstance, nil
	})

	// Make it a proper constructor with prototype property
	if ctorObj := syntaxErrorConstructor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties
		ctorWithProps := vm.NewNativeFunctionWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(syntaxErrorPrototype))

		syntaxErrorConstructor = ctorWithProps
	}

	// Set constructor property on prototype
	syntaxErrorPrototype.SetOwnNonEnumerable("constructor", syntaxErrorConstructor)

	// Define globally
	return ctx.DefineGlobal("SyntaxError", syntaxErrorConstructor)
}

// InitSyntaxError creates and returns a SyntaxErrorInitializer
func InitSyntaxError() BuiltinInitializer {
	return &SyntaxErrorInitializer{}
}
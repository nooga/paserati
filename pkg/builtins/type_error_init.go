package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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
	// Per ECMAScript 19.5.6.3.2, NativeError.prototype.message is the empty string
	typeErrorPrototype.SetOwnNonEnumerable("message", vm.NewString(""))

	// TypeError constructor function (length is 1 per ECMAScript 19.5.6.2)
	typeErrorConstructor := vm.NewNativeFunction(1, true, "TypeError", func(args []vm.Value) (vm.Value, error) {
		// Get message argument
		var message string
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			message = args[0].ToString()
		}

		// Create new TypeError instance that inherits from TypeError.prototype
		typeErrorInstance := vm.NewObject(vm.NewValueFromPlainObject(typeErrorPrototype))
		typeErrorInstancePtr := typeErrorInstance.AsPlainObject()

		// Set [[ErrorData]] internal slot (used by Error.isError to distinguish real errors)
		typeErrorInstancePtr.SetOwn("[[ErrorData]]", vm.Undefined)

		// Set properties (override name, set message and stack)
		typeErrorInstancePtr.SetOwnNonEnumerable("name", vm.NewString("TypeError"))
		typeErrorInstancePtr.SetOwnNonEnumerable("message", vm.NewString(message))

		// Capture stack trace at the time of TypeError creation
		stackTrace := vmInstance.CaptureStackTrace()
		typeErrorInstancePtr.SetOwnNonEnumerable("stack", vm.NewString(stackTrace))

		// Per ECMAScript 20.5.8.1 InstallErrorCause:
		// If options is an Object and HasProperty(options, "cause") is true,
		// install the cause property
		if len(args) > 1 && args[1].IsObject() {
			options := args[1]
			// Check if options has "cause" property (via prototype chain)
			if optObj := options.AsPlainObject(); optObj != nil {
				if cause, hasCause := optObj.Get("cause"); hasCause {
					typeErrorInstancePtr.SetOwnNonEnumerable("cause", cause)
				}
			}
		}

		return typeErrorInstance, nil
	})

	// Make it a proper constructor with prototype property
	if ctorObj := typeErrorConstructor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties
		ctorWithProps := vm.NewConstructorWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(typeErrorPrototype))

		// Per ECMAScript 19.5.6.2, the [[Prototype]] of a NativeError constructor is Error
		if !vmInstance.ErrorConstructor.IsUndefined() {
			ctorPropsObj.Properties.SetPrototype(vmInstance.ErrorConstructor)
		}

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

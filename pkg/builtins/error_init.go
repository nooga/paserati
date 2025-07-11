package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// ErrorInitializer implements the Error constructor and Error.prototype
type ErrorInitializer struct{}

func (e *ErrorInitializer) Name() string {
	return "Error"
}

func (e *ErrorInitializer) Priority() int {
	return 20 // After basic types but before utility objects
}

func (e *ErrorInitializer) InitTypes(ctx *TypeContext) error {
	// Create Error.prototype type
	errorProtoType := types.NewObjectType().
		WithProperty("name", types.String).
		WithProperty("message", types.String).
		WithProperty("stack", types.String).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String))

	// Create Error constructor type
	errorCtorType := types.NewObjectType().
		// Constructor is callable with optional message parameter
		WithSimpleCallSignature([]types.Type{}, errorProtoType).
		WithSimpleCallSignature([]types.Type{types.String}, errorProtoType).
		WithProperty("prototype", errorProtoType)

	// Define the constructor globally
	return ctx.DefineGlobal("Error", errorCtorType)
}

func (e *ErrorInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Error.prototype object
	errorPrototype := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	
	// Set Error.prototype.name = "Error"
	errorPrototype.SetOwn("name", vm.NewString("Error"))
	
	// Set Error.prototype.message = ""
	errorPrototype.SetOwn("message", vm.NewString(""))
	
	// Error.prototype.toString()
	errorPrototype.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		// Get 'this' context from VM
		thisValue := vmInstance.GetThis()
		
		// Default values
		name := "Error"
		message := ""
		
		// If 'this' is an object, try to get name and message properties
		if thisValue.IsObject() {
			if plainObj := thisValue.AsPlainObject(); plainObj != nil {
				if nameValue, exists := plainObj.GetOwn("name"); exists && nameValue.IsString() {
					name = nameValue.AsString()
				}
				if messageValue, exists := plainObj.GetOwn("message"); exists && messageValue.IsString() {
					message = messageValue.AsString()
				}
			} else if dictObj := thisValue.AsDictObject(); dictObj != nil {
				if nameValue, exists := dictObj.GetOwn("name"); exists && nameValue.IsString() {
					name = nameValue.AsString()
				}
				if messageValue, exists := dictObj.GetOwn("message"); exists && messageValue.IsString() {
					message = messageValue.AsString()
				}
			}
		}
		
		// Return "name: message" format, or just "name" if no message
		if message == "" {
			return vm.NewString(name), nil
		}
		return vm.NewString(name + ": " + message), nil
	}))
	
	// Error constructor function
	errorConstructor := vm.NewNativeFunction(-1, true, "Error", func(args []vm.Value) (vm.Value, error) {
		// Get message argument
		var message string
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			message = args[0].ToString()
		}
		
		// Create new Error instance
		errorInstance := vm.NewObject(vm.NewValueFromPlainObject(errorPrototype))
		errorInstancePtr := errorInstance.AsPlainObject()
		
		// Set properties
		errorInstancePtr.SetOwn("name", vm.NewString("Error"))
		errorInstancePtr.SetOwn("message", vm.NewString(message))
		
		// Capture stack trace at the time of Error creation
		stackTrace := vmInstance.CaptureStackTrace()
		errorInstancePtr.SetOwn("stack", vm.NewString(stackTrace))
		
		return errorInstance, nil
	})

	// Make it a proper constructor with prototype property
	if ctorObj := errorConstructor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties
		ctorWithProps := vm.NewNativeFunctionWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(errorPrototype))

		errorConstructor = ctorWithProps
	}

	// Set constructor property on prototype
	errorPrototype.SetOwn("constructor", errorConstructor)

	// Store in VM
	vmInstance.ErrorPrototype = vm.NewValueFromPlainObject(errorPrototype)
	
	// Define globally
	return ctx.DefineGlobal("Error", errorConstructor)
}

// InitError creates and returns an ErrorInitializer
func InitError() BuiltinInitializer {
	return &ErrorInitializer{}
}
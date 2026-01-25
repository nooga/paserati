package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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
	errorPrototype.SetOwnNonEnumerable("name", vm.NewString("Error"))

	// Set Error.prototype.message = ""
	errorPrototype.SetOwnNonEnumerable("message", vm.NewString(""))

	// Error.prototype.toString()
	errorPrototype.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
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
		errorInstancePtr.SetOwnNonEnumerable("name", vm.NewString("Error"))
		errorInstancePtr.SetOwnNonEnumerable("message", vm.NewString(message))

		// Capture stack trace at the time of Error creation
		stackTrace := vmInstance.CaptureStackTrace()
		errorInstancePtr.SetOwnNonEnumerable("stack", vm.NewString(stackTrace))

		return errorInstance, nil
	})

	// Make it a proper constructor with prototype property
	if ctorObj := errorConstructor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties
		ctorWithProps := vm.NewConstructorWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(errorPrototype))

		// Add Error.isError static method (ES2024)
		ctorPropsObj.Properties.SetOwnNonEnumerable("isError", vm.NewNativeFunction(1, false, "isError", func(args []vm.Value) (vm.Value, error) {
			if len(args) == 0 {
				return vm.BooleanValue(false), nil
			}
			arg := args[0]
			// Check if the value has Error.prototype in its prototype chain
			return vm.BooleanValue(isErrorValue(vmInstance, arg)), nil
		}))

		errorConstructor = ctorWithProps
	}

	// Set constructor property on prototype
	errorPrototype.SetOwnNonEnumerable("constructor", errorConstructor)
	// DEBUG (optional): we could print or assert here if needed

	// Store in VM
	vmInstance.ErrorPrototype = vm.NewValueFromPlainObject(errorPrototype)

	// Define globally
	return ctx.DefineGlobal("Error", errorConstructor)
}

// InitError creates and returns an ErrorInitializer
func InitError() BuiltinInitializer {
	return &ErrorInitializer{}
}

// EvalError
type EvalErrorInitializer struct{}

func (e *EvalErrorInitializer) Name() string  { return "EvalError" }
func (e *EvalErrorInitializer) Priority() int { return 22 }
func (e *EvalErrorInitializer) InitTypes(ctx *TypeContext) error {
	t := types.NewObjectType().WithSimpleCallSignature([]types.Type{}, types.Any).WithSimpleCallSignature([]types.Type{types.String}, types.Any)
	return ctx.DefineGlobal("EvalError", t)
}
func (e *EvalErrorInitializer) InitRuntime(ctx *RuntimeContext) error {
	return initErrorSubclass(ctx, "EvalError")
}

// RangeError
type RangeErrorInitializer struct{}

func (e *RangeErrorInitializer) Name() string  { return "RangeError" }
func (e *RangeErrorInitializer) Priority() int { return 22 }
func (e *RangeErrorInitializer) InitTypes(ctx *TypeContext) error {
	t := types.NewObjectType().WithSimpleCallSignature([]types.Type{}, types.Any).WithSimpleCallSignature([]types.Type{types.String}, types.Any)
	return ctx.DefineGlobal("RangeError", t)
}
func (e *RangeErrorInitializer) InitRuntime(ctx *RuntimeContext) error {
	return initErrorSubclass(ctx, "RangeError")
}

// URIError
type URIErrorInitializer struct{}

func (e *URIErrorInitializer) Name() string  { return "URIError" }
func (e *URIErrorInitializer) Priority() int { return 22 }
func (e *URIErrorInitializer) InitTypes(ctx *TypeContext) error {
	t := types.NewObjectType().WithSimpleCallSignature([]types.Type{}, types.Any).WithSimpleCallSignature([]types.Type{types.String}, types.Any)
	return ctx.DefineGlobal("URIError", t)
}
func (e *URIErrorInitializer) InitRuntime(ctx *RuntimeContext) error {
	return initErrorSubclass(ctx, "URIError")
}

// helper to initialize simple Error subclasses inheriting Error.prototype
func initErrorSubclass(ctx *RuntimeContext, name string) error {
	vmInstance := ctx.VM
	proto := vm.NewObject(vmInstance.ErrorPrototype).AsPlainObject()
	proto.SetOwnNonEnumerable("name", vm.NewString(name))
	ctor := vm.NewNativeFunction(-1, true, name, func(args []vm.Value) (vm.Value, error) {
		var message string
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			message = args[0].ToString()
		}
		inst := vm.NewObject(vm.NewValueFromPlainObject(proto)).AsPlainObject()
		inst.SetOwnNonEnumerable("name", vm.NewString(name))
		inst.SetOwnNonEnumerable("message", vm.NewString(message))
		inst.SetOwnNonEnumerable("stack", vm.NewString(vmInstance.CaptureStackTrace()))
		return vm.NewValueFromPlainObject(inst), nil
	})
	if nf := ctor.AsNativeFunction(); nf != nil {
		withProps := vm.NewConstructorWithProps(nf.Arity, nf.Variadic, nf.Name, nf.Fn)
		withProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(proto))
		proto.SetOwnNonEnumerable("constructor", withProps)
		return ctx.DefineGlobal(name, withProps)
	}
	proto.SetOwnNonEnumerable("constructor", ctor)
	return ctx.DefineGlobal(name, ctor)
}

// isErrorValue checks if a value is an Error instance (has Error.prototype in its chain)
func isErrorValue(vmInstance *vm.VM, val vm.Value) bool {
	// Must be an object-like value
	if !val.IsObject() {
		return false
	}

	// Get the Error.prototype to compare against
	errorProto := vmInstance.ErrorPrototype

	// Walk the prototype chain
	current := val
	for {
		var proto vm.Value

		// Get prototype based on object type
		if po := current.AsPlainObject(); po != nil {
			proto = po.GetPrototype()
		} else if d := current.AsDictObject(); d != nil {
			proto = d.GetPrototype()
		} else {
			// Not a valid object type for prototype chain walking
			return false
		}

		// End of prototype chain
		if proto.Type() == vm.TypeNull || proto.Type() == vm.TypeUndefined {
			return false
		}

		// Check if this prototype is Error.prototype
		if proto.Type() == errorProto.Type() {
			// Compare the actual objects
			if protoPO := proto.AsPlainObject(); protoPO != nil {
				if errorProtoPO := errorProto.AsPlainObject(); errorProtoPO != nil {
					if protoPO == errorProtoPO {
						return true
					}
				}
			}
		}

		current = proto
	}
}

package builtins

import (
	"fmt"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// FunctionInitializer implements the Function builtin
type FunctionInitializer struct{}

func (f *FunctionInitializer) Name() string {
	return "Function"
}

func (f *FunctionInitializer) Priority() int {
	return PriorityFunction // Must be after Object but before others
}

func (f *FunctionInitializer) InitTypes(ctx *TypeContext) error {
	// Create Function.prototype type using fluent API
	functionProtoType := types.NewObjectType().
		WithProperty("length", types.Number). // Number of parameters (excluding rest and defaults after first optional)
		WithProperty("call", types.NewVariadicFunction([]types.Type{}, types.Any, &types.ArrayType{ElementType: types.Any})).
		WithProperty("apply", types.NewSimpleFunction([]types.Type{types.Any, &types.ArrayType{ElementType: types.Any}}, types.Any)).
		WithProperty("bind", types.NewVariadicFunction([]types.Type{types.Any}, types.Any, &types.ArrayType{ElementType: types.Any})).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String))

	// Create Function constructor type using fluent API
	functionCtorType := types.NewObjectType().
		// Constructor is callable (Function constructor creates functions from strings)
		WithSimpleCallSignature([]types.Type{}, types.Any).                 // Function() -> function
		WithVariadicCallSignature([]types.Type{}, types.Any, types.String). // Function(...args, body) -> function
		// Static properties
		WithProperty("prototype", functionProtoType)

	// Define the constructor globally
	if err := ctx.DefineGlobal("Function", functionCtorType); err != nil {
		return err
	}

	// Store the prototype type for primitive "function"
	ctx.SetPrimitivePrototype("function", functionProtoType)

	return nil
}

func (f *FunctionInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Function.prototype as a callable function (per ECMAScript spec)
	// Function.prototype is a function that accepts any arguments and returns undefined
	functionProtoFn := vm.NewNativeFunctionWithProps(0, true, "", func(args []vm.Value) (vm.Value, error) {
		// Per spec: Function.prototype() returns undefined
		return vm.Undefined, nil
	})
	functionProtoObj := functionProtoFn.AsNativeFunctionWithProps()

	// Set prototype chain: Function.prototype.[[Prototype]] = Object.prototype
	functionProtoObj.Properties.SetPrototype(ctx.ObjectPrototype)

	// Add prototype methods
	// Function.prototype.call
	callImpl := func(args []vm.Value) (vm.Value, error) {
		return functionPrototypeCallImpl(vmInstance, args)
	}
	functionProtoObj.Properties.SetOwnNonEnumerable("call", vm.NewNativeFunction(0, true, "call", callImpl))

	// Function.prototype.apply
	applyImpl := func(args []vm.Value) (vm.Value, error) {
		return functionPrototypeApplyImpl(vmInstance, args)
	}
	functionProtoObj.Properties.SetOwnNonEnumerable("apply", vm.NewNativeFunction(2, false, "apply", applyImpl))

	// Function.prototype.bind
	bindImpl := func(args []vm.Value) (vm.Value, error) {
		return functionPrototypeBindImpl(vmInstance, args)
	}
	functionProtoObj.Properties.SetOwnNonEnumerable("bind", vm.NewNativeFunction(0, true, "bind", bindImpl))

	// Function.prototype.toString
	toStringImpl := func(args []vm.Value) (vm.Value, error) {
		return functionPrototypeToStringImpl(vmInstance, args)
	}
	functionProtoObj.Properties.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", toStringImpl))

	// Create Function constructor
	functionCtor := vm.NewNativeFunction(-1, true, "Function", func(args []vm.Value) (vm.Value, error) {
		return functionConstructorImpl(vmInstance, ctx.Driver, args)
	})

	// Make it a proper constructor with static methods
	if ctorObj := functionCtor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties
		ctorWithProps := vm.NewConstructorWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwnNonEnumerable("prototype", functionProtoFn)

		functionCtor = ctorWithProps
	}

	// Set constructor property on prototype
	functionProtoObj.Properties.SetOwnNonEnumerable("constructor", functionCtor)

	// Store in VM - Function.prototype is now a callable
	vmInstance.FunctionPrototype = functionProtoFn

	// Define globally
	return ctx.DefineGlobal("Function", functionCtor)
}

// Implementation methods

func functionPrototypeCallImpl(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	// Get 'this' function from VM context
	thisFunction := vmInstance.GetThis()

	if !thisFunction.IsCallable() {
		return vm.Undefined, fmt.Errorf("TypeError: %v is not a function", thisFunction.Type())
	}

	// Extract thisArg and function arguments
	var thisArg vm.Value = vm.Undefined
	var functionArgs []vm.Value

	if len(args) > 0 {
		thisArg = args[0]
		functionArgs = args[1:]
	}

	// Use the unified Call method to handle all function types properly
	return vmInstance.Call(thisFunction, thisArg, functionArgs)
}

func functionPrototypeApplyImpl(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	// Get 'this' function from VM context
	thisFunction := vmInstance.GetThis()

	if !thisFunction.IsCallable() {
		return vm.Undefined, fmt.Errorf("TypeError: %v is not a function", thisFunction.Type())
	}

	// Extract thisArg and arguments array
	var thisArg vm.Value = vm.Undefined
	var functionArgs []vm.Value

	if len(args) > 0 {
		thisArg = args[0]
	}

	if len(args) > 1 {
		argsArray := args[1]
		// Handle array-like arguments
		if argsArray.IsArray() {
			arrayObj := argsArray.AsArray()
			functionArgs = make([]vm.Value, arrayObj.Length())
			for i := 0; i < arrayObj.Length(); i++ {
				functionArgs[i] = arrayObj.Get(i)
			}
		} else if !argsArray.IsUndefined() {
			// TODO: Handle array-like objects (with length property)
			// For now, treat non-array as empty arguments
			functionArgs = []vm.Value{}
		}
	}

	// Use the unified Call method to handle all function types properly
	return vmInstance.Call(thisFunction, thisArg, functionArgs)
}

func functionPrototypeBindImpl(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	// Get 'this' from VM instead of first argument
	originalFunc := vmInstance.GetThis()

	if !originalFunc.IsCallable() {
		return vm.Undefined, fmt.Errorf("TypeError: %v is not a function", originalFunc.Type())
	}

	var boundThis vm.Value = vm.Undefined
	var partialArgs []vm.Value

	if len(args) > 0 {
		boundThis = args[0]    // First argument is the 'this' value to bind to
		partialArgs = args[1:] // Remaining arguments are partial arguments
	}

	// Create function name for debugging
	functionName := "bound"
	switch originalFunc.Type() {
	case vm.TypeNativeFunction:
		if nativeFunc := originalFunc.AsNativeFunction(); nativeFunc != nil {
			functionName = "bound " + nativeFunc.Name
		}
	case vm.TypeNativeFunctionWithProps:
		if nativeFuncWithProps := originalFunc.AsNativeFunctionWithProps(); nativeFuncWithProps != nil {
			functionName = "bound " + nativeFuncWithProps.Name
		}
	case vm.TypeFunction:
		if fn := originalFunc.AsFunction(); fn != nil {
			functionName = "bound " + fn.Name
		}
	case vm.TypeClosure:
		if closure := originalFunc.AsClosure(); closure != nil && closure.Fn != nil {
			functionName = "bound " + closure.Fn.Name
		}
	case vm.TypeBoundFunction:
		if boundFn := originalFunc.AsBoundFunction(); boundFn != nil {
			functionName = "bound " + boundFn.Name
		}
	}

	// Create a bound function using the new BoundFunction type
	result := vm.NewBoundFunction(originalFunc, boundThis, partialArgs, functionName)
	return result, nil
}

func functionPrototypeToStringImpl(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	// Get 'this' function from VM context
	thisFunction := vmInstance.GetThis()

	if !thisFunction.IsCallable() {
		return vm.Undefined, fmt.Errorf("TypeError: %v is not a function", thisFunction.Type())
	}

	// Return a string representation of the function
	var name string
	switch thisFunction.Type() {
	case vm.TypeNativeFunction:
		nf := thisFunction.AsNativeFunction()
		name = nf.Name
		if name == "" {
			name = "anonymous"
		}
		return vm.NewString(fmt.Sprintf("function %s() { [native code] }", name)), nil
	case vm.TypeNativeFunctionWithProps:
		nfp := thisFunction.AsNativeFunctionWithProps()
		name = nfp.Name
		if name == "" {
			name = "anonymous"
		}
		return vm.NewString(fmt.Sprintf("function %s() { [native code] }", name)), nil
	case vm.TypeFunction:
		fn := thisFunction.AsFunction()
		name = fn.Name
		if name == "" {
			name = "anonymous"
		}
		return vm.NewString(fmt.Sprintf("function %s() { [bytecode] }", name)), nil
	case vm.TypeClosure:
		cl := thisFunction.AsClosure()
		if cl.Fn != nil {
			name = cl.Fn.Name
		}
		if name == "" {
			name = "anonymous"
		}
		return vm.NewString(fmt.Sprintf("function %s() { [bytecode] }", name)), nil
	case vm.TypeBoundFunction:
		bf := thisFunction.AsBoundFunction()
		name = bf.Name
		if name == "" {
			name = "bound anonymous"
		}
		return vm.NewString(fmt.Sprintf("function %s() { [bound] }", name)), nil
	default:
		return vm.NewString("function() { [unknown] }"), nil
	}
}

func functionConstructorImpl(vmInstance *vm.VM, driver interface{}, args []vm.Value) (vm.Value, error) {
	// The Function constructor has signature:
	// Function(param1, param2, ..., paramN, body)
	// Where all arguments are strings

	var params []string
	var body string

	if len(args) == 0 {
		// Function() - no parameters, empty body
		body = ""
	} else if len(args) == 1 {
		// Function(body) - no parameters, just body
		body = args[0].ToString()
	} else {
		// Function(param1, ..., paramN, body) - last arg is body, rest are parameters
		for i := 0; i < len(args)-1; i++ {
			params = append(params, args[i].ToString())
		}
		body = args[len(args)-1].ToString()
	}

	// Construct the function source code as an IIFE that returns the function
	// This ensures it's evaluated as an expression
	var source string
	if len(params) == 0 {
		source = fmt.Sprintf("return (function() { %s });", body)
	} else {
		// Join parameters with commas
		paramStr := ""
		for i, param := range params {
			if i > 0 {
				paramStr += ", "
			}
			paramStr += param
		}
		source = fmt.Sprintf("return (function(%s) { %s });", paramStr, body)
	}

	// We need access to the driver to compile this source code
	// IMPORTANT: We use CompileProgram instead of RunString to avoid corrupting
	// the parent compilation's state (EnableModuleMode, etc.)
	if driver == nil {
		return vm.Undefined, fmt.Errorf("SyntaxError: Function constructor - driver is nil")
	}

	// Define interface for accessing compiler without state modification
	type driverInterface interface {
		CompileProgram(*parser.Program) (*vm.Chunk, []errors.PaseratiError)
	}

	d, ok := driver.(driverInterface)
	if !ok {
		return vm.Undefined, fmt.Errorf("SyntaxError: Function constructor - driver doesn't implement CompileProgram (got type %T)", driver)
	}

	// Parse the source code
	lx := lexer.NewLexer(source)
	p := parser.NewParser(lx)
	prog, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return vm.Undefined, fmt.Errorf("SyntaxError: %v", parseErrs[0])
	}

	// Compile the program (this uses the existing compiler without modifying its state)
	chunk, compileErrs := d.CompileProgram(prog)
	if len(compileErrs) > 0 {
		return vm.Undefined, fmt.Errorf("SyntaxError: %v", compileErrs[0])
	}

	if chunk == nil {
		return vm.Undefined, fmt.Errorf("SyntaxError: compilation returned nil chunk")
	}

	// IMPORTANT: Don't execute the chunk! The compiled code is:
	//   return (function(...) { body });
	// This generates bytecode:
	//   OpLoadConst R1, Constant[0]  // Constant[0] is the function object
	//   OpReturn R1
	// We can skip execution and just return constants[0] directly.

	if len(chunk.Constants) == 0 {
		return vm.Undefined, fmt.Errorf("Internal Error: compiled Function() code has no constants")
	}

	// The first constant is the function object we created
	functionValue := chunk.Constants[0]

	// Verify it's actually a function
	if !functionValue.IsCallable() {
		return vm.Undefined, fmt.Errorf("Internal Error: Function() constant is not callable (got %s)", functionValue.TypeName())
	}

	return functionValue, nil
}

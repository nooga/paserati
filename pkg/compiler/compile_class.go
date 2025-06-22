package compiler

import (
	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

// compileClassDeclaration compiles a class declaration into a constructor function + prototype setup
// This follows the approach of desugaring classes to constructor functions + prototypes
func (c *Compiler) compileClassDeclaration(node *parser.ClassDeclaration, hint Register) (Register, errors.PaseratiError) {
	debugPrintf("// DEBUG compileClassDeclaration: Starting compilation for class '%s'\n", node.Name.Value)
	
	// 1. Create constructor function
	constructorReg, err := c.compileConstructor(node)
	if err != nil {
		return BadRegister, err
	}
	
	// 2. Set up prototype object with methods
	err = c.setupClassPrototype(node, constructorReg)
	if err != nil {
		return BadRegister, err
	}
	
	// 3. Define the class name in the symbol table (maps to constructor function)
	c.currentSymbolTable.Define(node.Name.Value, constructorReg)
	
	debugPrintf("// DEBUG compileClassDeclaration: Successfully compiled class '%s' to R%d\n", node.Name.Value, constructorReg)
	return constructorReg, nil
}

// compileConstructor creates a constructor function from the class constructor method
func (c *Compiler) compileConstructor(node *parser.ClassDeclaration) (Register, errors.PaseratiError) {
	// Find the constructor method in the class body
	var constructorMethod *parser.MethodDefinition
	for _, method := range node.Body.Methods {
		if method.Kind == "constructor" {
			constructorMethod = method
			break
		}
	}
	
	// Create function literal for the constructor
	var functionLiteral *parser.FunctionLiteral
	if constructorMethod != nil {
		// Use the existing constructor method
		functionLiteral = constructorMethod.Value
	} else {
		// Create default constructor
		functionLiteral = c.createDefaultConstructor(node)
	}
	
	// Compile the constructor function
	nameHint := node.Name.Value
	funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(functionLiteral, nameHint)
	if err != nil {
		return BadRegister, err
	}
	
	// Create closure for constructor
	constructorReg := c.regAlloc.Alloc()
	c.emitClosure(constructorReg, funcConstIndex, functionLiteral, freeSymbols)
	
	debugPrintf("// DEBUG compileConstructor: Constructor compiled to R%d\n", constructorReg)
	return constructorReg, nil
}

// createDefaultConstructor creates a default constructor function when none is provided
func (c *Compiler) createDefaultConstructor(node *parser.ClassDeclaration) *parser.FunctionLiteral {
	// Create empty parameter list
	parameters := []*parser.Parameter{}
	
	// Create empty block statement for body
	body := &parser.BlockStatement{
		Token:      node.Token,
		Statements: []parser.Statement{},
	}
	
	// Create function literal
	return &parser.FunctionLiteral{
		Token:      node.Token,
		Name:       nil, // Anonymous constructor
		Parameters: parameters,
		Body:       body,
	}
}

// setupClassPrototype sets up the prototype object with class methods
func (c *Compiler) setupClassPrototype(node *parser.ClassDeclaration, constructorReg Register) errors.PaseratiError {
	debugPrintf("// DEBUG setupClassPrototype: Setting up prototype for class '%s'\n", node.Name.Value)
	
	// Create prototype object
	prototypeReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(prototypeReg)
	c.emitMakeEmptyObject(prototypeReg, node.Token.Line)
	
	// Add methods to prototype (excluding constructor and static methods)
	for _, method := range node.Body.Methods {
		if method.Kind != "constructor" && !method.IsStatic {
			err := c.addMethodToPrototype(method, prototypeReg)
			if err != nil {
				return err
			}
		}
	}
	
	// Set constructor.prototype = prototypeObject
	prototypeNameIdx := c.chunk.AddConstant(vm.String("prototype"))
	c.emitSetProp(constructorReg, prototypeReg, prototypeNameIdx, node.Token.Line)
	
	// Set prototypeObject.constructor = constructor
	constructorNameIdx := c.chunk.AddConstant(vm.String("constructor"))
	c.emitSetProp(prototypeReg, constructorReg, constructorNameIdx, node.Token.Line)
	
	debugPrintf("// DEBUG setupClassPrototype: Prototype setup complete for class '%s'\n", node.Name.Value)
	return nil
}

// addMethodToPrototype compiles a method and adds it to the prototype object
func (c *Compiler) addMethodToPrototype(method *parser.MethodDefinition, prototypeReg Register) errors.PaseratiError {
	debugPrintf("// DEBUG addMethodToPrototype: Adding method '%s' to prototype\n", method.Key.Value)
	
	// Compile the method function
	nameHint := method.Key.Value
	funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(method.Value, nameHint)
	if err != nil {
		return err
	}
	
	// Create closure for method
	methodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(methodReg)
	c.emitClosure(methodReg, funcConstIndex, method.Value, freeSymbols)
	
	// Add method to prototype: prototype[methodName] = methodFunction
	methodNameIdx := c.chunk.AddConstant(vm.String(method.Key.Value))
	c.emitSetProp(prototypeReg, methodReg, methodNameIdx, method.Token.Line)
	
	debugPrintf("// DEBUG addMethodToPrototype: Method '%s' added to prototype\n", method.Key.Value)
	return nil
}
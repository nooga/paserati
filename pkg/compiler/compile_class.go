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
	
	// 3. Set up static members on the constructor
	err = c.setupStaticMembers(node, constructorReg)
	if err != nil {
		return BadRegister, err
	}
	
	// 4. Store the class constructor globally
	if c.enclosing == nil {
		// Top-level class declaration - define as global
		globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
		c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
		c.emitSetGlobal(globalIdx, constructorReg, node.Token.Line)
		debugPrintf("// DEBUG compileClassDeclaration: Defined global class '%s' at index %d\n", node.Name.Value, globalIdx)
	} else {
		// Local class declaration (inside function/block)
		c.currentSymbolTable.Define(node.Name.Value, constructorReg)
		debugPrintf("// DEBUG compileClassDeclaration: Defined local class '%s' in R%d\n", node.Name.Value, constructorReg)
	}
	
	// Class declarations don't produce a value for the hint register
	return BadRegister, nil
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
	
	// Inject field initializers into the constructor body
	functionLiteral = c.injectFieldInitializers(node, functionLiteral)
	
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

// injectFieldInitializers creates a new function literal with field initializers prepended to the constructor body
func (c *Compiler) injectFieldInitializers(node *parser.ClassDeclaration, functionLiteral *parser.FunctionLiteral) *parser.FunctionLiteral {
	// Collect field initializer statements
	var fieldInitializers []parser.Statement
	
	// Extract field initializers from class properties
	for _, property := range node.Body.Properties {
		if property.Value != nil { // Property has an initializer
			// Create assignment statement: this.propertyName = initializerExpression
			assignment := &parser.AssignmentExpression{
				Token:    property.Token,
				Operator: "=",
				Left: &parser.MemberExpression{
					Token:    property.Token,
					Object:   &parser.ThisExpression{Token: property.Token},
					Property: property.Key,
				},
				Value: property.Value,
			}
			
			// Wrap in expression statement
			fieldInitStatement := &parser.ExpressionStatement{
				Token:      property.Token,
				Expression: assignment,
			}
			
			fieldInitializers = append(fieldInitializers, fieldInitStatement)
			debugPrintf("// DEBUG injectFieldInitializers: Added field initializer for '%s'\n", property.Key.Value)
		}
	}
	
	// If no field initializers, return original function literal
	if len(fieldInitializers) == 0 {
		return functionLiteral
	}
	
	// Create new body with field initializers prepended to original statements
	newStatements := make([]parser.Statement, 0, len(fieldInitializers)+len(functionLiteral.Body.Statements))
	newStatements = append(newStatements, fieldInitializers...)
	newStatements = append(newStatements, functionLiteral.Body.Statements...)
	
	// Create new function literal with modified body
	newFunctionLiteral := &parser.FunctionLiteral{
		Token:                functionLiteral.Token,
		Name:                 functionLiteral.Name,
		TypeParameters:       functionLiteral.TypeParameters,
		Parameters:           functionLiteral.Parameters,
		RestParameter:        functionLiteral.RestParameter,
		ReturnTypeAnnotation: functionLiteral.ReturnTypeAnnotation,
		Body: &parser.BlockStatement{
			Token:      functionLiteral.Body.Token,
			Statements: newStatements,
		},
	}
	
	debugPrintf("// DEBUG injectFieldInitializers: Created constructor with %d field initializers\n", len(fieldInitializers))
	return newFunctionLiteral
}

// setupStaticMembers sets up static properties and methods on the constructor function
func (c *Compiler) setupStaticMembers(node *parser.ClassDeclaration, constructorReg Register) errors.PaseratiError {
	debugPrintf("// DEBUG setupStaticMembers: Setting up static members for class '%s'\n", node.Name.Value)
	
	// Add static properties
	for _, property := range node.Body.Properties {
		if property.IsStatic {
			err := c.addStaticProperty(property, constructorReg)
			if err != nil {
				return err
			}
		}
	}
	
	// Add static methods
	for _, method := range node.Body.Methods {
		if method.IsStatic && method.Kind != "constructor" {
			err := c.addStaticMethod(method, constructorReg)
			if err != nil {
				return err
			}
		}
	}
	
	debugPrintf("// DEBUG setupStaticMembers: Static members setup complete for class '%s'\n", node.Name.Value)
	return nil
}

// addStaticProperty compiles a static property and adds it to the constructor
func (c *Compiler) addStaticProperty(property *parser.PropertyDefinition, constructorReg Register) errors.PaseratiError {
	debugPrintf("// DEBUG addStaticProperty: Adding static property '%s'\n", property.Key.Value)
	
	// Allocate a register for the property value
	valueReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(valueReg)
	
	// Compile the property value (if it has an initializer)
	if property.Value != nil {
		var err errors.PaseratiError
		compiledReg, err := c.compileNode(property.Value, valueReg)
		if err != nil {
			return err
		}
		// If compileNode returned a different register, move it to our allocated register
		if compiledReg != valueReg {
			c.emitMove(valueReg, compiledReg, property.Token.Line)
			c.regAlloc.Free(compiledReg)
		}
	} else {
		// No initializer, use undefined
		c.emitLoadUndefined(valueReg, property.Token.Line)
	}
	
	// Set constructor[propertyName] = value
	propertyNameIdx := c.chunk.AddConstant(vm.String(property.Key.Value))
	c.emitSetProp(constructorReg, valueReg, propertyNameIdx, property.Token.Line)
	
	debugPrintf("// DEBUG addStaticProperty: Static property '%s' added to constructor\n", property.Key.Value)
	return nil
}

// addStaticMethod compiles a static method and adds it to the constructor
func (c *Compiler) addStaticMethod(method *parser.MethodDefinition, constructorReg Register) errors.PaseratiError {
	debugPrintf("// DEBUG addStaticMethod: Adding static method '%s'\n", method.Key.Value)
	
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
	
	// Set constructor[methodName] = methodFunction
	methodNameIdx := c.chunk.AddConstant(vm.String(method.Key.Value))
	c.emitSetProp(constructorReg, methodReg, methodNameIdx, method.Token.Line)
	
	debugPrintf("// DEBUG addStaticMethod: Static method '%s' added to constructor\n", method.Key.Value)
	return nil
}
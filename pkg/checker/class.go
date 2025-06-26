package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// checkClassDeclaration handles type checking for class declarations
func (c *Checker) checkClassDeclaration(node *parser.ClassDeclaration) {
	debugPrintf("// [Checker Class] Checking class declaration '%s'\n", node.Name.Value)
	fmt.Printf("DEBUG: Checking class declaration '%s'\n", node.Name.Value)
	
	// 1. Check if class name is already defined
	if _, _, exists := c.env.Resolve(node.Name.Value); exists {
		c.addError(node.Name, fmt.Sprintf("identifier '%s' already declared", node.Name.Value))
		return
	}
	
	// 2. Create instance type from methods and properties
	instanceType := c.createInstanceType(node.Body)
	
	// 3. Register the class name as a type alias pointing to the instance type EARLY
	// This allows static methods to use the class name in type annotations
	if !c.env.DefineTypeAlias(node.Name.Value, instanceType) {
		c.addError(node.Name, fmt.Sprintf("failed to define class type '%s'", node.Name.Value))
		return
	}
	debugPrintf("// [Checker Class] Early defined class type alias '%s': %s\n", 
		node.Name.Value, instanceType.String())
	
	// 4. Create constructor signature from constructor method  
	constructorSig := c.createConstructorSignature(node.Body, instanceType)
	
	// 5. Create constructor function type (ObjectType with construct signature)
	// This represents the class constructor function, not a separate "class type"
	constructorType := types.NewConstructorType(constructorSig)
	
	// 6. Add static members to the constructor type (can now resolve class name in type annotations)
	constructorType = c.addStaticMembers(node.Body, constructorType)
	
	// 6. Register the constructor function in the environment
	// When we reference "Animal", we get the constructor function, not a separate class type
	if !c.env.Define(node.Name.Value, constructorType, false) {
		c.addError(node.Name, fmt.Sprintf("failed to define class '%s'", node.Name.Value))
		return
	}
	
	debugPrintf("// [Checker Class] Successfully defined class '%s' as constructor type: %s\n", 
		node.Name.Value, constructorType.String())
	fmt.Printf("DEBUG: Successfully defined class '%s' as constructor type: %s\n", 
		node.Name.Value, constructorType.String())
}

// createInstanceType builds an object type representing instances of the class
func (c *Checker) createInstanceType(body *parser.ClassBody) *types.ObjectType {
	instanceType := types.NewObjectType()
	
	// Add methods to instance type (excluding constructor and static methods)
	for _, method := range body.Methods {
		if method.Kind != "constructor" && !method.IsStatic {
			methodType := c.inferMethodType(method)
			instanceType = instanceType.WithProperty(method.Key.Value, methodType)
			debugPrintf("// [Checker Class] Added method '%s' to instance type: %s\n", 
				method.Key.Value, methodType.String())
		}
	}
	
	// Add properties to instance type (excluding static properties)
	for _, prop := range body.Properties {
		if !prop.IsStatic {
			propType := c.inferPropertyType(prop)
			instanceType = instanceType.WithProperty(prop.Key.Value, propType)
			debugPrintf("// [Checker Class] Added property '%s' to instance type: %s\n", 
				prop.Key.Value, propType.String())
		}
	}
	
	return instanceType
}

// createConstructorSignature creates a signature for the class constructor
func (c *Checker) createConstructorSignature(body *parser.ClassBody, instanceType *types.ObjectType) *types.Signature {
	// Find the constructor method
	var constructor *parser.MethodDefinition
	for _, method := range body.Methods {
		if method.Kind == "constructor" {
			constructor = method
			break
		}
	}
	
	if constructor == nil {
		// Default constructor: no parameters, returns the instance type
		return &types.Signature{
			ParameterTypes:    []types.Type{},
			ReturnType:        instanceType,
			OptionalParams:    []bool{},
			IsVariadic:        false,
			RestParameterType: nil,
		}
	}
	
	// Extract parameter types from constructor
	paramTypes := c.extractParameterTypes(constructor.Value)
	optionalParams := c.extractOptionalParams(constructor.Value)
	
	return &types.Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        instanceType,
		OptionalParams:    optionalParams,
		IsVariadic:        constructor.Value.RestParameter != nil,
		RestParameterType: c.extractRestParameterType(constructor.Value),
	}
}

// inferMethodType determines the type of a class method
func (c *Checker) inferMethodType(method *parser.MethodDefinition) types.Type {
	if method.Value == nil {
		debugPrintf("// [Checker Class] Method '%s' has no function value, using Any\n", method.Key.Value)
		return types.Any
	}
	
	// Extract parameter types and return type from the function
	paramTypes := c.extractParameterTypes(method.Value)
	returnType := c.inferReturnType(method.Value)
	optionalParams := c.extractOptionalParams(method.Value)
	
	signature := &types.Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    optionalParams,
		IsVariadic:        method.Value.RestParameter != nil,
		RestParameterType: c.extractRestParameterType(method.Value),
	}
	
	return types.NewFunctionType(signature)
}

// inferPropertyType determines the type of a class property
func (c *Checker) inferPropertyType(prop *parser.PropertyDefinition) types.Type {
	if prop.Value == nil {
		// Property without initializer gets 'any' type for now
		// In a full implementation, this might be 'undefined' or require explicit type annotation
		return types.Any
	}
	
	// Type check the initializer expression to get its type
	c.visit(prop.Value)
	if initType := prop.Value.GetComputedType(); initType != nil {
		return initType
	}
	
	return types.Any
}

// addStaticMembers adds static methods and properties to the constructor type
func (c *Checker) addStaticMembers(body *parser.ClassBody, constructorType *types.ObjectType) *types.ObjectType {
	// Add static methods
	for _, method := range body.Methods {
		if method.IsStatic && method.Kind != "constructor" {
			methodType := c.inferMethodType(method)
			constructorType = constructorType.WithProperty(method.Key.Value, methodType)
			debugPrintf("// [Checker Class] Added static method '%s' to constructor type: %s\n", 
				method.Key.Value, methodType.String())
		}
	}
	
	// Add static properties
	for _, prop := range body.Properties {
		if prop.IsStatic {
			propType := c.inferPropertyType(prop)
			constructorType = constructorType.WithProperty(prop.Key.Value, propType)
			if prop.Optional {
				constructorType.OptionalProperties[prop.Key.Value] = true
			}
			debugPrintf("// [Checker Class] Added static property '%s' to constructor type: %s\n", 
				prop.Key.Value, propType.String())
		}
	}
	
	return constructorType
}

// extractParameterTypes extracts types from function parameters
func (c *Checker) extractParameterTypes(fn *parser.FunctionLiteral) []types.Type {
	var paramTypes []types.Type
	
	for _, param := range fn.Parameters {
		if param.TypeAnnotation != nil {
			// Use explicit type annotation
			paramType := c.resolveTypeAnnotation(param.TypeAnnotation)
			if paramType != nil {
				paramTypes = append(paramTypes, paramType)
			} else {
				paramTypes = append(paramTypes, types.Any)
			}
		} else {
			// No type annotation, use 'any'
			paramTypes = append(paramTypes, types.Any)
		}
	}
	
	return paramTypes
}

// extractOptionalParams determines which parameters are optional
func (c *Checker) extractOptionalParams(fn *parser.FunctionLiteral) []bool {
	optionalParams := make([]bool, len(fn.Parameters))
	
	for i, param := range fn.Parameters {
		optionalParams[i] = param.Optional
	}
	
	return optionalParams
}

// extractRestParameterType gets the type of the rest parameter if present
func (c *Checker) extractRestParameterType(fn *parser.FunctionLiteral) types.Type {
	if fn.RestParameter == nil {
		return nil
	}
	
	if fn.RestParameter.TypeAnnotation != nil {
		// Use explicit type annotation
		restType := c.resolveTypeAnnotation(fn.RestParameter.TypeAnnotation)
		if restType != nil {
			return restType
		}
	}
	
	// Default to any[] for rest parameters
	return &types.ArrayType{ElementType: types.Any}
}

// inferReturnType determines the return type of a function
func (c *Checker) inferReturnType(fn *parser.FunctionLiteral) types.Type {
	if fn.ReturnTypeAnnotation != nil {
		// Use explicit return type annotation
		returnType := c.resolveTypeAnnotation(fn.ReturnTypeAnnotation)
		if returnType != nil {
			return returnType
		}
	}
	
	// For now, return 'any' if no explicit type annotation
	// In a full implementation, we would analyze the function body to infer the return type
	return types.Any
}
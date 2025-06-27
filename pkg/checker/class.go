package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// checkClassDeclaration handles type checking for class declarations
func (c *Checker) checkClassDeclaration(node *parser.ClassDeclaration) {
	debugPrintf("// [Checker Class] Checking class declaration '%s'\n", node.Name.Value)
	
	// Set up class context for access control checking
	prevContext := c.currentClassContext
	c.setClassContext(node.Name.Value, types.AccessContextExternal)
	defer func() { c.currentClassContext = prevContext }()
	
	// 1. Check if class name is already defined
	if _, _, exists := c.env.Resolve(node.Name.Value); exists {
		c.addError(node.Name, fmt.Sprintf("identifier '%s' already declared", node.Name.Value))
		return
	}
	
	// 2. Create instance type from methods and properties
	instanceType := c.createInstanceType(node.Name.Value, node.Body)
	
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
}

// createInstanceType builds an object type representing instances of the class
func (c *Checker) createInstanceType(className string, body *parser.ClassBody) *types.ObjectType {
	// Create class instance type with metadata
	instanceType := types.NewClassInstanceType(className)
	
	// Add methods to instance type (excluding constructor and static methods)
	for _, method := range body.Methods {
		if method.Kind != "constructor" && !method.IsStatic {
			// Set method context for access control checking
			c.setClassContext(className, types.AccessContextInstanceMethod)
			
			methodType := c.inferMethodType(method)
			
			// Determine access level
			accessLevel := c.getAccessLevel(method.IsPublic, method.IsPrivate, method.IsProtected)
			
			// Add method with access control metadata
			instanceType.WithClassMember(method.Key.Value, methodType, accessLevel, false, false)
			
			debugPrintf("// [Checker Class] Added method '%s' to instance type: %s (%s)\n", 
				method.Key.Value, methodType.String(), accessLevel.String())
		}
	}
	
	// Add properties to instance type (excluding static properties)
	for _, prop := range body.Properties {
		if !prop.IsStatic {
			propType := c.inferPropertyType(prop)
			
			// Determine access level
			accessLevel := c.getAccessLevel(prop.IsPublic, prop.IsPrivate, prop.IsProtected)
			
			// Add property with access control metadata
			instanceType.WithClassMember(prop.Key.Value, propType, accessLevel, false, prop.Readonly)
			
			debugPrintf("// [Checker Class] Added property '%s' to instance type: %s (%s, readonly: %v)\n", 
				prop.Key.Value, propType.String(), accessLevel.String(), prop.Readonly)
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
	var propType types.Type
	
	// First check if there's an explicit type annotation
	if prop.TypeAnnotation != nil {
		annotationType := c.resolveTypeAnnotation(prop.TypeAnnotation)
		if annotationType != nil {
			propType = annotationType
		} else {
			propType = types.Any
		}
	} else if prop.Value != nil {
		// Type check the initializer expression to get its type
		c.visit(prop.Value)
		if initType := prop.Value.GetComputedType(); initType != nil {
			propType = initType
		} else {
			propType = types.Any
		}
	} else {
		// Property without initializer or type annotation gets 'any' type for now
		propType = types.Any
	}
	
	// Wrap with readonly if the property is readonly
	if prop.Readonly {
		return types.NewReadonlyType(propType)
	}
	
	return propType
}

// getAccessLevel determines the access level from boolean flags
func (c *Checker) getAccessLevel(isPublic, isPrivate, isProtected bool) types.AccessModifier {
	if isPrivate {
		return types.AccessPrivate
	}
	if isProtected {
		return types.AccessProtected
	}
	// Default to public if no explicit modifier (TypeScript default)
	return types.AccessPublic
}

// addStaticMembers adds static methods and properties to the constructor type
func (c *Checker) addStaticMembers(body *parser.ClassBody, constructorType *types.ObjectType) *types.ObjectType {
	// Get the class name from constructor metadata
	className := constructorType.GetClassName()
	
	// Add static methods
	for _, method := range body.Methods {
		if method.IsStatic && method.Kind != "constructor" {
			// Set static method context for access control checking
			c.setClassContext(className, types.AccessContextStaticMethod)
			
			methodType := c.inferMethodType(method)
			
			// Determine access level
			accessLevel := c.getAccessLevel(method.IsPublic, method.IsPrivate, method.IsProtected)
			
			// Add method with access control metadata
			constructorType.WithClassMember(method.Key.Value, methodType, accessLevel, true, false)
			
			debugPrintf("// [Checker Class] Added static method '%s' to constructor type: %s (%s)\n", 
				method.Key.Value, methodType.String(), accessLevel.String())
		}
	}
	
	// Add static properties
	for _, prop := range body.Properties {
		if prop.IsStatic {
			propType := c.inferPropertyType(prop)
			
			// Determine access level
			accessLevel := c.getAccessLevel(prop.IsPublic, prop.IsPrivate, prop.IsProtected)
			
			// Add property with access control metadata
			constructorType.WithClassMember(prop.Key.Value, propType, accessLevel, true, prop.Readonly)
			
			// Handle optional properties
			if prop.Optional {
				constructorType.OptionalProperties[prop.Key.Value] = true
			}
			
			debugPrintf("// [Checker Class] Added static property '%s' to constructor type: %s (%s, readonly: %v)\n", 
				prop.Key.Value, propType.String(), accessLevel.String(), prop.Readonly)
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
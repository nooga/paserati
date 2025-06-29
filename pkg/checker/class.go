package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// extractPropertyName extracts the property name from a class member key
func (c *Checker) extractPropertyName(key parser.Expression) string {
	switch k := key.(type) {
	case *parser.Identifier:
		return k.Value
	case *parser.ComputedPropertyName:
		// For computed properties, try to evaluate the expression at compile time if possible
		// For now, use a placeholder name
		if ident, ok := k.Expr.(*parser.Identifier); ok {
			return fmt.Sprintf("__computed_%s", ident.Value)
		} else if literal, ok := k.Expr.(*parser.StringLiteral); ok {
			return literal.Value
		} else if literal, ok := k.Expr.(*parser.NumberLiteral); ok {
			return fmt.Sprintf("%v", literal.Value)
		} else {
			return fmt.Sprintf("__computed_%p", k.Expr)
		}
	default:
		return fmt.Sprintf("__unknown_%p", key)
	}
}

// checkClassDeclaration handles type checking for class declarations
func (c *Checker) checkClassDeclaration(node *parser.ClassDeclaration) {
	debugPrintf("// [Checker Class] Checking class declaration '%s'\n", node.Name.Value)

	// Handle generic classes
	if len(node.TypeParameters) > 0 {
		c.checkGenericClassDeclaration(node)
		return
	}

	// Track abstract classes for instantiation validation
	if node.IsAbstract {
		c.abstractClasses[node.Name.Value] = true
		debugPrintf("// [Checker Class] Marked class '%s' as abstract\n", node.Name.Value)
	}

	// Set up class context for access control checking
	prevContext := c.currentClassContext
	c.setClassContext(node.Name.Value, types.AccessContextExternal)
	defer func() { c.currentClassContext = prevContext }()

	// 1. Check if class name is already defined
	if _, _, exists := c.env.Resolve(node.Name.Value); exists {
		c.addError(node.Name, fmt.Sprintf("identifier '%s' already declared", node.Name.Value))
		return
	}

	// 1.5. EARLY DEFINITION: Create a forward reference type and define it early
	// This allows methods to reference the class constructor during processing
	forwardRefType := &types.ForwardReferenceType{
		ClassName:      node.Name.Value,
		TypeParameters: nil, // Non-generic class
	}
	c.env.Define(node.Name.Value, forwardRefType, false)
	debugPrintf("// [Checker Class] Early defined forward reference for non-generic class '%s'\n", node.Name.Value)

	// 2. Handle inheritance relationships and create instance type from methods and properties
	instanceType := c.createInstanceType(node.Name.Value, node.Body, node.SuperClass, node.Implements)

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

	// 6. Update the constructor function in the environment (replacing forward reference)
	// When we reference "Animal", we get the constructor function, not a separate class type
	if !c.env.Update(node.Name.Value, constructorType) {
		c.addError(node.Name, fmt.Sprintf("failed to update class '%s'", node.Name.Value))
		return
	}

	debugPrintf("// [Checker Class] Successfully defined class '%s' as constructor type: %s\n",
		node.Name.Value, constructorType.String())
}

// checkGenericClassDeclaration handles generic class declarations
func (c *Checker) checkGenericClassDeclaration(node *parser.ClassDeclaration) {
	debugPrintf("// [Checker Class] Processing generic class '%s' with %d type parameters\n",
		node.Name.Value, len(node.TypeParameters))

	// 1. Validate type parameters
	typeParams := make([]*types.TypeParameter, len(node.TypeParameters))
	for i, param := range node.TypeParameters {
		// Create TypeParameter type
		typeParam := &types.TypeParameter{
			Name: param.Name.Value,
		}

		// Handle constraint if present
		if param.Constraint != nil {
			constraintType := c.resolveTypeAnnotation(param.Constraint)
			if constraintType != nil {
				typeParam.Constraint = constraintType
			}
		}

		typeParams[i] = typeParam
	}

	// 2. Create a new environment with type parameters available as TypeParameterType
	genericEnv := NewEnclosedEnvironment(c.env)
	for _, typeParam := range typeParams {
		paramType := &types.TypeParameterType{
			Parameter: typeParam,
		}
		genericEnv.DefineTypeAlias(typeParam.Name, paramType)
	}

	// Create a forward reference type for self-references during class processing
	forwardRefType := &types.ForwardReferenceType{
		ClassName:      node.Name.Value,
		TypeParameters: typeParams,
	}

	// EARLY DEFINITION: Define the class name in the OUTER environment first
	// This allows the final update to work correctly
	savedEnv := c.env
	savedEnv.Define(node.Name.Value, forwardRefType, false)
	debugPrintf("// [Checker Class] Early defined forward reference for '%s' in outer environment\n", node.Name.Value)

	// Switch to generic environment
	c.env = genericEnv

	// Also define in the generic environment for method type checking
	genericEnv.Define(node.Name.Value, forwardRefType, false)
	debugPrintf("// [Checker Class] Early defined forward reference for '%s' in generic environment\n", node.Name.Value)

	// Set as current forward reference for self-references during class processing
	savedCurrentGenericClass := c.currentGenericClass
	savedCurrentForwardRef := c.currentForwardRef
	c.currentGenericClass = nil
	c.currentForwardRef = forwardRefType

	// Track abstract classes for instantiation validation (in generic env)
	if node.IsAbstract {
		c.abstractClasses[node.Name.Value] = true
		debugPrintf("// [Checker Class] Marked generic class '%s' as abstract\n", node.Name.Value)
	}

	// 3. Create the instance type body with TypeParameterType references
	instanceType := c.createInstanceType(node.Name.Value, node.Body, node.SuperClass, node.Implements)

	// 4. Create constructor signature from constructor method
	constructorSig := c.createConstructorSignature(node.Body, instanceType)

	// 5. Create constructor type
	constructorType := types.NewConstructorType(constructorSig)
	constructorType = c.addStaticMembers(node.Body, constructorType)

	// Restore environment and current state
	c.env = savedEnv
	c.currentGenericClass = savedCurrentGenericClass
	c.currentForwardRef = savedCurrentForwardRef

	// 6. Create the GenericType for the class
	genericClassType := &types.GenericType{
		Name:           node.Name.Value,
		TypeParameters: typeParams,
		Body:           instanceType,
	}

	// 7. Create a GenericType for the constructor as well
	// Note: The constructor GenericType is for internal use only
	// It should not appear in user-facing error messages
	genericConstructorType := &types.GenericType{
		Name:           node.Name.Value + "Constructor", // Internal name
		TypeParameters: typeParams,
		Body:           constructorType,
	}

	// 8. Update in environment - store the generic constructor type (replacing forward reference)
	if !c.env.Update(node.Name.Value, genericConstructorType) {
		c.addError(node.Name, fmt.Sprintf("failed to update generic class '%s'", node.Name.Value))
		return
	}

	// Also define the class type alias for type annotations
	if !c.env.DefineTypeAlias(node.Name.Value, genericClassType) {
		debugPrintf("// [Checker Class] WARNING: DefineTypeAlias failed for generic class '%s'.\n", node.Name.Value)
	}

	debugPrintf("// [Checker Class] Successfully defined generic class '%s' with %d type parameters\n",
		node.Name.Value, len(typeParams))
}

// createInstanceType builds an object type representing instances of the class
func (c *Checker) createInstanceType(className string, body *parser.ClassBody, superClass parser.Expression, implements []*parser.Identifier) *types.ObjectType {
	// Create class instance type with metadata
	instanceType := types.NewClassInstanceType(className)

	// Handle inheritance relationships
	if superClass != nil {
		c.handleClassInheritance(instanceType, superClass)
	}

	// Handle interface implementations
	for _, iface := range implements {
		c.handleInterfaceImplementation(instanceType, iface.Value)
	}

	// Collect getters and setters to handle them specially
	getters := make(map[string]*parser.MethodDefinition)
	setters := make(map[string]*parser.MethodDefinition)

	// First pass: collect getters and setters
	for _, method := range body.Methods {
		if method.Kind == "getter" && !method.IsStatic {
			getters[c.extractPropertyName(method.Key)] = method
		} else if method.Kind == "setter" && !method.IsStatic {
			setters[c.extractPropertyName(method.Key)] = method
		}
	}

	// Second pass: handle getters and setters to create properties
	propertyNames := make(map[string]bool)
	for name := range getters {
		propertyNames[name] = true
	}
	for name := range setters {
		propertyNames[name] = true
	}

	for propName := range propertyNames {
		getter := getters[propName]
		setter := setters[propName]

		var propType types.Type
		var accessLevel types.AccessModifier

		// Determine property type and access level
		if getter != nil {
			c.setClassContext(className, types.AccessContextInstanceMethod)

			// Validate override keyword usage for getter
			if getter.IsOverride {
				c.validateOverrideMethod(getter, superClass, className)
			}

			methodType := c.inferMethodType(getter)
			if objType, ok := methodType.(*types.ObjectType); ok && len(objType.CallSignatures) > 0 {
				propType = objType.CallSignatures[0].ReturnType
			}
			accessLevel = c.getAccessLevel(getter.IsPublic, getter.IsPrivate, getter.IsProtected)
			debugPrintf("// [Checker Class] Found getter for property '%s': %s (%s)\n",
				propName, propType.String(), accessLevel.String())
		}

		if setter != nil {
			c.setClassContext(className, types.AccessContextInstanceMethod)

			// Validate override keyword usage for setter
			if setter.IsOverride {
				c.validateOverrideMethod(setter, superClass, className)
			}

			methodType := c.inferMethodType(setter)
			if objType, ok := methodType.(*types.ObjectType); ok && len(objType.CallSignatures) > 0 {
				sig := objType.CallSignatures[0]
				if len(sig.ParameterTypes) > 0 {
					setterType := sig.ParameterTypes[0]
					if propType == nil {
						propType = setterType
					}
					// Use the more restrictive access level
					setterAccessLevel := c.getAccessLevel(setter.IsPublic, setter.IsPrivate, setter.IsProtected)
					if accessLevel == types.AccessPublic {
						accessLevel = setterAccessLevel
					}
					debugPrintf("// [Checker Class] Found setter for property '%s': %s (%s)\n",
						propName, setterType.String(), setterAccessLevel.String())
				}
			}
		}

		if propType != nil {
			instanceType.WithClassMember(propName, propType, accessLevel, false, false)
			debugPrintf("// [Checker Class] Added getter/setter property '%s' to instance type: %s (%s)\n",
				propName, propType.String(), accessLevel.String())
		}
	}

	// Track computed properties for index signatures
	var hasComputedProperties bool
	var computedValueTypes []types.Type

	// Add regular methods to instance type (excluding constructor, static methods, getters, and setters)
	for _, method := range body.Methods {
		if method.Kind != "constructor" && !method.IsStatic && method.Kind != "getter" && method.Kind != "setter" {
			// Set method context for access control checking
			c.setClassContext(className, types.AccessContextInstanceMethod)

			// Validate override keyword usage
			if method.IsOverride {
				c.validateOverrideMethod(method, superClass, className)
			}

			methodType := c.inferMethodType(method)

			// Determine access level
			accessLevel := c.getAccessLevel(method.IsPublic, method.IsPrivate, method.IsProtected)

			// Check if this is a computed property
			if _, isComputed := method.Key.(*parser.ComputedPropertyName); isComputed {
				// For computed methods, add to index signature instead of individual property
				hasComputedProperties = true
				computedValueTypes = append(computedValueTypes, methodType)
				debugPrintf("// [Checker Class] Found computed method, adding to index signature: %s\n", methodType.String())
			} else {
				// Add method with access control metadata
				methodName := c.extractPropertyName(method.Key)
				instanceType.WithClassMember(methodName, methodType, accessLevel, false, false)

				debugPrintf("// [Checker Class] Added method '%s' to instance type: %s (%s)\n",
					methodName, methodType.String(), accessLevel.String())
			}
		}
	}

	// Process method signatures (overloads) - for now, we'll validate they exist but not add them separately
	// In a full implementation, we'd create overloaded function types
	for _, methodSig := range body.MethodSigs {
		if !methodSig.IsStatic {
			c.setClassContext(className, types.AccessContextInstanceMethod)

			// Validate override keyword usage for method signature
			if methodSig.IsOverride {
				c.validateOverrideMethodSignature(methodSig, superClass, className)
			}

			// Validate the signature types
			c.validateMethodSignature(methodSig)

			debugPrintf("// [Checker Class] Validated method signature '%s'\n", c.extractPropertyName(methodSig.Key))
		}
	}

	// Add properties to instance type (excluding static properties)
	for _, prop := range body.Properties {
		if !prop.IsStatic {
			propType := c.inferPropertyType(prop)

			// Determine access level
			accessLevel := c.getAccessLevel(prop.IsPublic, prop.IsPrivate, prop.IsProtected)

			// Check if this is a computed property
			if _, isComputed := prop.Key.(*parser.ComputedPropertyName); isComputed {
				// For computed properties, add to index signature instead of individual property
				hasComputedProperties = true
				computedValueTypes = append(computedValueTypes, propType)
				debugPrintf("// [Checker Class] Found computed property, adding to index signature: %s\n", propType.String())
			} else {
				// Add property with access control metadata
				propName := c.extractPropertyName(prop.Key)
				instanceType.WithClassMember(propName, propType, accessLevel, false, prop.Readonly)

				debugPrintf("// [Checker Class] Added property '%s' to instance type: %s (%s, readonly: %v)\n",
					propName, propType.String(), accessLevel.String(), prop.Readonly)
			}
		}
	}

	// If class has computed properties, create index signatures
	if hasComputedProperties {
		// Create a union type of all computed property value types
		var indexValueType types.Type
		if len(computedValueTypes) == 1 {
			indexValueType = computedValueTypes[0]
		} else if len(computedValueTypes) > 1 {
			indexValueType = &types.UnionType{Types: computedValueTypes}
		} else {
			indexValueType = types.Any
		}

		// Add index signature: [key: string]: ComputedValueType
		instanceType.IndexSignatures = []*types.IndexSignature{
			{
				KeyType:   types.String,
				ValueType: indexValueType,
			},
		}

		debugPrintf("// [Checker Class] Added index signature for computed properties: [key: string]: %s\n", indexValueType.String())
	}

	return instanceType
}

// validateOverrideMethod validates the usage of the override keyword
func (c *Checker) validateOverrideMethod(method *parser.MethodDefinition, superClass parser.Expression, className string) {
	// Basic validation: if there's no superclass, override doesn't make sense
	methodName := c.extractPropertyName(method.Key)
	if superClass == nil {
		c.addError(method.Key, fmt.Sprintf("method '%s' uses 'override' but class '%s' does not extend any class", methodName, className))
		return
	}

	// TODO: When inheritance is implemented, add:
	// 1. Check if the method exists in the superclass
	// 2. Check if the method signatures are compatible
	// 3. Check if the method is not final/sealed
	// 4. Check access modifier compatibility

	debugPrintf("// [Checker Class] Override validation for method '%s' in class '%s' (inheritance not yet implemented)\n",
		methodName, className)
}

// validateOverrideMethodSignature validates the usage of the override keyword for method signatures
func (c *Checker) validateOverrideMethodSignature(methodSig *parser.MethodSignature, superClass parser.Expression, className string) {
	// Basic validation: if there's no superclass, override doesn't make sense
	methodName := c.extractPropertyName(methodSig.Key)
	if superClass == nil {
		c.addError(methodSig.Key, fmt.Sprintf("method signature '%s' uses 'override' but class '%s' does not extend any class", methodName, className))
		return
	}

	// TODO: When inheritance is implemented, add similar validation as for method definitions

	debugPrintf("// [Checker Class] Override validation for method signature '%s' in class '%s' (inheritance not yet implemented)\n",
		methodName, className)
}

// createConstructorSignature creates a signature for the class constructor
// Always uses the implementation signature for runtime, while overload signatures are for compile-time checking
func (c *Checker) createConstructorSignature(body *parser.ClassBody, instanceType *types.ObjectType) *types.Signature {
	// Find the constructor method implementation first (this is what's used at runtime)
	var constructor *parser.MethodDefinition
	for _, method := range body.Methods {
		if method.Kind == "constructor" {
			constructor = method
			break
		}
	}

	if constructor != nil {
		// Extract parameter types from constructor implementation
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

	// If no implementation but there are signatures, use the first signature
	// This handles the case where only signatures are provided (which should be an error)
	if len(body.ConstructorSigs) > 0 {
		firstSig := body.ConstructorSigs[0]
		paramTypes := c.extractParameterTypesFromSignature(firstSig)
		optionalParams := c.extractOptionalParamsFromSignature(firstSig)

		return &types.Signature{
			ParameterTypes:    paramTypes,
			ReturnType:        instanceType,
			OptionalParams:    optionalParams,
			IsVariadic:        firstSig.RestParameter != nil,
			RestParameterType: c.extractRestParameterTypeFromSignature(firstSig),
		}
	}

	// Default constructor: no parameters, returns the instance type
	return &types.Signature{
		ParameterTypes:    []types.Type{},
		ReturnType:        instanceType,
		OptionalParams:    []bool{},
		IsVariadic:        false,
		RestParameterType: nil,
	}
}

// inferMethodType determines the type of a class method
func (c *Checker) inferMethodType(method *parser.MethodDefinition) types.Type {
	if method.Value == nil {
		methodName := c.extractPropertyName(method.Key)
		debugPrintf("// [Checker Class] Method '%s' has no function value, using Any\n", methodName)
		return types.Any
	}

	// For generic methods, we need to check them properly to make type parameters available
	if len(method.Value.TypeParameters) > 0 {
		// Check the method's function literal which will handle type parameters
		c.checkFunctionLiteral(method.Value)
		
		// Get the computed type from the function literal
		if computedType := method.Value.GetComputedType(); computedType != nil {
			return computedType
		}
		// Fallback if checking failed
		return types.Any
	}

	// Extract parameter types and return type from the function
	paramTypes := c.extractParameterTypes(method.Value)
	returnType := c.inferReturnType(method.Value)
	optionalParams := c.extractOptionalParams(method.Value)
	isVariadic := method.Value.RestParameter != nil
	restParameterType := c.extractRestParameterType(method.Value)

	signature := &types.Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    optionalParams,
		IsVariadic:        isVariadic,
		RestParameterType: restParameterType,
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
			methodName := c.extractPropertyName(method.Key)
			constructorType.WithClassMember(methodName, methodType, accessLevel, true, false)

			debugPrintf("// [Checker Class] Added static method '%s' to constructor type: %s (%s)\n",
				methodName, methodType.String(), accessLevel.String())
		}
	}

	// Add static properties
	for _, prop := range body.Properties {
		if prop.IsStatic {
			propType := c.inferPropertyType(prop)

			// Determine access level
			accessLevel := c.getAccessLevel(prop.IsPublic, prop.IsPrivate, prop.IsProtected)

			// Add property with access control metadata
			propName := c.extractPropertyName(prop.Key)
			constructorType.WithClassMember(propName, propType, accessLevel, true, prop.Readonly)

			// Handle optional properties
			if prop.Optional {
				constructorType.OptionalProperties[propName] = true
			}

			debugPrintf("// [Checker Class] Added static property '%s' to constructor type: %s (%s, readonly: %v)\n",
				propName, propType.String(), accessLevel.String(), prop.Readonly)
		}
	}

	return constructorType
}

// extractParameterTypes extracts types from function parameters
func (c *Checker) extractParameterTypes(fn *parser.FunctionLiteral) []types.Type {
	var paramTypes []types.Type

	// If the function has type parameters, create a temporary environment with them
	originalEnv := c.env
	if len(fn.TypeParameters) > 0 {
		typeParamEnv := NewEnclosedEnvironment(c.env)
		
		// Define each type parameter in the environment
		for i, typeParamNode := range fn.TypeParameters {
			// Create the type parameter
			typeParam := &types.TypeParameter{
				Name:       typeParamNode.Name.Value,
				Constraint: types.Any, // Simple constraint for now
				Index:      i,
			}
			
			// Create a type parameter type
			typeParamType := &types.TypeParameterType{
				Parameter: typeParam,
			}
			
			// Define in the environment
			typeParamEnv.DefineTypeAlias(typeParam.Name, typeParamType)
		}
		
		// Use the type param environment for resolving parameter types
		c.env = typeParamEnv
	}

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

	// Restore original environment
	c.env = originalEnv

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
		// If the function has type parameters, create a temporary environment with them
		originalEnv := c.env
		if len(fn.TypeParameters) > 0 {
			typeParamEnv := NewEnclosedEnvironment(c.env)
			
			// Define each type parameter in the environment
			for i, typeParamNode := range fn.TypeParameters {
				// Create the type parameter
				typeParam := &types.TypeParameter{
					Name:       typeParamNode.Name.Value,
					Constraint: types.Any, // Simple constraint for now
					Index:      i,
				}
				
				// Create a type parameter type
				typeParamType := &types.TypeParameterType{
					Parameter: typeParam,
				}
				
				// Define in the environment
				typeParamEnv.DefineTypeAlias(typeParam.Name, typeParamType)
			}
			
			// Use the type param environment for resolving rest parameter type
			c.env = typeParamEnv
		}

		// Use explicit type annotation
		restType := c.resolveTypeAnnotation(fn.RestParameter.TypeAnnotation)
		
		// Restore original environment
		c.env = originalEnv
		
		if restType != nil {
			return restType
		}
	}

	// Default to any[] for rest parameters
	return &types.ArrayType{ElementType: types.Any}
}

// handleClassInheritance processes class inheritance (extends clause)
func (c *Checker) handleClassInheritance(instanceType *types.ObjectType, superClassExpr parser.Expression) {
	debugPrintf("// [Checker Class] === STARTING handleClassInheritance: %s extends %s ===\n", instanceType.GetClassName(), superClassExpr.String())

	// Resolve the superclass type expression (supports both simple names and generic applications)
	superType := c.resolveTypeAnnotation(superClassExpr)
	if superType == nil {
		c.addError(superClassExpr, "failed to resolve superclass type")
		return
	}

	// Handle different superclass types
	var constructorType types.Type
	var exists bool
	
	debugPrintf("// [Checker Class] Analyzing superclass expression type: %T\n", superClassExpr)
	
	// If it's a simple identifier, try to resolve it as a variable (class constructor)
	if ident, ok := superClassExpr.(*parser.Identifier); ok {
		debugPrintf("// [Checker Class] Superclass is simple identifier: %s\n", ident.Value)
		constructorType, _, exists = c.env.Resolve(ident.Value)
		if !exists {
			c.addError(superClassExpr, fmt.Sprintf("superclass '%s' is not defined", ident.Value))
			return
		}
	} else if genRef, ok := superClassExpr.(*parser.GenericTypeRef); ok {
		// For generic type applications like Container<T>, we need to:
		// 1. Resolve the base type as a constructor
		// 2. Instantiate it with the given type arguments
		debugPrintf("// [Checker Class] Processing GenericTypeRef for '%s'\n", genRef.Name.Value)
		debugPrintf("// [Checker Class] Looking for generic base constructor '%s'\n", genRef.Name.Value)
		baseConstructor, _, resolveExists := c.env.Resolve(genRef.Name.Value)
		if !resolveExists {
			debugPrintf("// [Checker Class] Generic base constructor '%s' not found in environment\n", genRef.Name.Value)
			c.addError(superClassExpr, fmt.Sprintf("generic superclass '%s' is not defined", genRef.Name.Value))
			return
		}
		debugPrintf("// [Checker Class] Found generic base constructor '%s': %T\n", genRef.Name.Value, baseConstructor)
		
		// TODO: Properly instantiate the generic constructor with type arguments
		// For now, use the base constructor (this is a simplification)
		constructorType = baseConstructor
		exists = true
		debugPrintf("// [Checker Class] Set constructorType and exists=true for GenericTypeRef\n")
	} else {
		// For other expressions, try to use the resolved type
		// This might not work for all cases but provides a fallback
		debugPrintf("// [Checker Class] Processing 'else' branch for superclass type\n")
		constructorType = superType
		exists = true
	}
	
	debugPrintf("// [Checker Class] About to check exists flag: exists=%t\n", exists)
	if !exists {
		debugPrintf("// [Checker Class] ERROR: exists = false after constructor resolution\n")
		c.addError(superClassExpr, "superclass is not defined")
		return
	}
	debugPrintf("// [Checker Class] Constructor resolution successful, exists = true\n")

	// Verify constructorType is a constructor (class)
	var superObjType *types.ObjectType
	var ok bool
	
	// Handle both ObjectType (regular classes) and GenericType (generic classes)
	switch ct := constructorType.(type) {
	case *types.ObjectType:
		superObjType = ct
		ok = true
	case *types.GenericType:
		// For generic classes, we need to use the constructor from the generic
		// For now, we'll accept the generic type and extract its constructor later
		// TODO: Properly instantiate the generic constructor with type arguments
		debugPrintf("// [Checker Class] Constructor is a GenericType, accepting as valid class\n")
		// Create a placeholder ObjectType for now
		superObjType = types.NewObjectType()
		ok = true
	default:
		ok = false
	}
	
	if !ok {
		c.addError(superClassExpr, fmt.Sprintf("'%s' is not a class and cannot be extended", superClassExpr.String()))
		return
	}

	// Check if it has constructor signatures (indicating it's a class constructor)
	// Skip this check for generic types for now
	if constructorType, ok := constructorType.(*types.ObjectType); ok {
		if len(constructorType.ConstructSignatures) == 0 {
			c.addError(superClassExpr, fmt.Sprintf("'%s' is not a class and cannot be extended", superClassExpr.String()))
			return
		}
	}

	// For simple identifiers, we can get the instance type by name
	// For generic applications, the superType should already be the instance type
	var superInstanceType *types.ObjectType
	if ident, ok := superClassExpr.(*parser.Identifier); ok {
		superInstanceType = c.getClassInstanceType(ident.Value)
		if superInstanceType == nil {
			c.addError(superClassExpr, fmt.Sprintf("could not find instance type for superclass '%s'", ident.Value))
			return
		}
	} else {
		// For generic applications, use the resolved type as the instance type
		if instanceType, ok := superType.(*types.ObjectType); ok {
			superInstanceType = instanceType
		} else {
			// For GenericType, use the placeholder we created
			superInstanceType = superObjType
		}
	}

	// Set inheritance relationship in class metadata
	if instanceType.ClassMeta != nil {
		// For simple inheritance, use the identifier name; for generic, use the string representation
		superClassName := superClassExpr.String()
		if ident, ok := superClassExpr.(*parser.Identifier); ok {
			superClassName = ident.Value
		}
		instanceType.ClassMeta.SetSuperClass(superClassName)
	}

	// Add superclass to BaseTypes for structural inheritance
	instanceType.BaseTypes = append(instanceType.BaseTypes, superInstanceType)

	debugPrintf("// [Checker Class] Successfully set up inheritance: %s extends %s\n", instanceType.GetClassName(), superClassExpr.String())
}

// handleInterfaceImplementation processes interface implementation (implements clause)
func (c *Checker) handleInterfaceImplementation(instanceType *types.ObjectType, interfaceName string) {
	debugPrintf("// [Checker Class] Handling interface implementation: %s implements %s\n", instanceType.GetClassName(), interfaceName)

	// Check if interface exists
	interfaceType, _, exists := c.env.Resolve(interfaceName)
	if !exists {
		c.addError(nil, fmt.Sprintf("interface '%s' is not defined", interfaceName))
		return
	}

	// Verify interfaceType is an interface (ObjectType without constructor signatures)
	interfaceObjType, ok := interfaceType.(*types.ObjectType)
	if !ok {
		c.addError(nil, fmt.Sprintf("'%s' is not an interface and cannot be implemented", interfaceName))
		return
	}

	// Add interface to class metadata
	if instanceType.ClassMeta != nil {
		instanceType.ClassMeta.AddImplementedInterface(interfaceName)
	}

	// Add interface to BaseTypes for structural inheritance
	instanceType.BaseTypes = append(instanceType.BaseTypes, interfaceObjType)

	// Validate that the class actually implements the interface
	c.validateInterfaceImplementation(instanceType, interfaceObjType, interfaceName)

	debugPrintf("// [Checker Class] Successfully set up interface implementation: %s implements %s\n", instanceType.GetClassName(), interfaceName)
}

// getClassInstanceType retrieves the instance type for a given class name
func (c *Checker) getClassInstanceType(className string) *types.ObjectType {
	// Try to resolve class type alias first
	if classType, exists := c.env.ResolveType(className); exists {
		if objType, ok := classType.(*types.ObjectType); ok && objType.IsClassInstance() {
			return objType
		}
	}
	return nil
}

// checkSuperExpression validates super expressions and determines their type
func (c *Checker) checkSuperExpression(node *parser.SuperExpression) {
	debugPrintf("// [Checker SuperExpr] Checking super expression\n")

	// Super is only valid within a class context
	if c.currentClassContext == nil {
		c.addError(node, "super expression can only be used within a class")
		node.SetComputedType(types.Any)
		return
	}

	// Get the current class metadata
	currentClass := c.currentClassContext.CurrentClassName
	classInstanceType := c.getClassInstanceType(currentClass)
	if classInstanceType == nil || classInstanceType.ClassMeta == nil {
		c.addError(node, "super expression can only be used within a class context")
		node.SetComputedType(types.Any)
		return
	}

	// Check if the current class has a superclass
	superClassName := classInstanceType.ClassMeta.SuperClassName
	if superClassName == "" {
		c.addError(node, fmt.Sprintf("class '%s' does not extend any class", currentClass))
		node.SetComputedType(types.Any)
		return
	}

	// Get the superclass instance type
	superInstanceType := c.getClassInstanceType(superClassName)
	if superInstanceType == nil {
		c.addError(node, fmt.Sprintf("could not resolve superclass '%s'", superClassName))
		node.SetComputedType(types.Any)
		return
	}

	// Set the computed type to the superclass instance type
	// This allows super.method() and super() calls to work correctly
	node.SetComputedType(superInstanceType)
	debugPrintf("// [Checker SuperExpr] Super expression type: %s\n", superInstanceType.String())
}

// validateInterfaceImplementation checks that a class properly implements an interface
func (c *Checker) validateInterfaceImplementation(classType *types.ObjectType, interfaceType *types.ObjectType, interfaceName string) {
	className := classType.GetClassName()
	debugPrintf("// [Checker Class] Validating interface implementation: %s implements %s\n", className, interfaceName)

	// Check that all interface properties are implemented
	for propName, propType := range interfaceType.Properties {
		classProperty, hasProp := classType.Properties[propName]
		if !hasProp {
			c.addError(nil, fmt.Sprintf("class '%s' is missing property '%s' required by interface '%s'", className, propName, interfaceName))
			continue
		}

		// Check type compatibility
		if !types.IsAssignable(classProperty, propType) {
			c.addError(nil, fmt.Sprintf("property '%s' in class '%s' is not compatible with interface '%s' (expected %s, got %s)",
				propName, className, interfaceName, propType.String(), classProperty.String()))
		}
	}

	// Check that all interface call signatures are implemented
	// For classes, this typically means checking methods
	for _, interfaceSig := range interfaceType.CallSignatures {
		// For method implementations, we need to check if the class has compatible methods
		// This is a simplified check - a full implementation would need more sophisticated matching
		debugPrintf("// [Checker Class] Interface %s requires call signature: %s\n", interfaceName, interfaceSig.String())
	}

	debugPrintf("// [Checker Class] Interface implementation validation completed for %s implements %s\n", className, interfaceName)
}

// inferReturnType determines the return type of a function
func (c *Checker) inferReturnType(fn *parser.FunctionLiteral) types.Type {
	if fn.ReturnTypeAnnotation != nil {
		// If the function has type parameters, create a temporary environment with them
		originalEnv := c.env
		if len(fn.TypeParameters) > 0 {
			typeParamEnv := NewEnclosedEnvironment(c.env)
			
			// Define each type parameter in the environment
			for i, typeParamNode := range fn.TypeParameters {
				// Create the type parameter
				typeParam := &types.TypeParameter{
					Name:       typeParamNode.Name.Value,
					Constraint: types.Any, // Simple constraint for now
					Index:      i,
				}
				
				// Create a type parameter type
				typeParamType := &types.TypeParameterType{
					Parameter: typeParam,
				}
				
				// Define in the environment
				typeParamEnv.DefineTypeAlias(typeParam.Name, typeParamType)
			}
			
			// Use the type param environment for resolving return type
			c.env = typeParamEnv
		}

		// Use explicit return type annotation
		returnType := c.resolveTypeAnnotation(fn.ReturnTypeAnnotation)
		
		// Restore original environment
		c.env = originalEnv
		
		if returnType != nil {
			return returnType
		}
	}

	// For now, return 'any' if no explicit type annotation
	// In a full implementation, we would analyze the function body to infer the return type
	return types.Any
}

// extractParameterTypesFromSignature extracts parameter types from a constructor signature
func (c *Checker) extractParameterTypesFromSignature(sig *parser.ConstructorSignature) []types.Type {
	var paramTypes []types.Type

	for _, param := range sig.Parameters {
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

// extractOptionalParamsFromSignature extracts optional parameter flags from a constructor signature
func (c *Checker) extractOptionalParamsFromSignature(sig *parser.ConstructorSignature) []bool {
	optionalParams := make([]bool, len(sig.Parameters))

	for i, param := range sig.Parameters {
		optionalParams[i] = param.Optional
	}

	return optionalParams
}

// extractRestParameterTypeFromSignature extracts rest parameter type from a constructor signature
func (c *Checker) extractRestParameterTypeFromSignature(sig *parser.ConstructorSignature) types.Type {
	if sig.RestParameter == nil {
		return nil
	}

	if sig.RestParameter.TypeAnnotation != nil {
		// Use explicit type annotation
		restType := c.resolveTypeAnnotation(sig.RestParameter.TypeAnnotation)
		if restType != nil {
			return restType
		}
	}

	// Default to any[] for rest parameters
	return &types.ArrayType{ElementType: types.Any}
}

// validateMethodSignature validates a method signature's types
func (c *Checker) validateMethodSignature(sig *parser.MethodSignature) {
	// Validate parameter types
	for _, param := range sig.Parameters {
		if param.TypeAnnotation != nil {
			paramType := c.resolveTypeAnnotation(param.TypeAnnotation)
			if paramType == nil {
				c.addError(param.Name, fmt.Sprintf("invalid type annotation for parameter '%s'", param.Name.Value))
			}
		}
	}

	// Validate rest parameter type if present
	if sig.RestParameter != nil && sig.RestParameter.TypeAnnotation != nil {
		restType := c.resolveTypeAnnotation(sig.RestParameter.TypeAnnotation)
		if restType == nil {
			c.addError(sig.RestParameter.Name, fmt.Sprintf("invalid type annotation for rest parameter '%s'", sig.RestParameter.Name.Value))
		}
	}

	// Validate return type if present
	if sig.ReturnTypeAnnotation != nil {
		returnType := c.resolveTypeAnnotation(sig.ReturnTypeAnnotation)
		if returnType == nil {
			methodName := c.extractPropertyName(sig.Key)
			c.addError(sig.Key, fmt.Sprintf("invalid return type annotation for method '%s'", methodName))
		}
	}
}

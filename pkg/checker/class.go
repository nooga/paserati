package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

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

	// 6. Register the constructor function in the environment
	// When we reference "Animal", we get the constructor function, not a separate class type
	if !c.env.Define(node.Name.Value, constructorType, false) {
		c.addError(node.Name, fmt.Sprintf("failed to define class '%s'", node.Name.Value))
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

	// Save current environment and switch to generic environment
	savedEnv := c.env
	c.env = genericEnv

	// Create a forward reference type for self-references during class processing
	forwardRefType := &types.ForwardReferenceType{
		ClassName:      node.Name.Value,
		TypeParameters: typeParams,
	}

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

	// 8. Define in environment - store the generic constructor type
	if !c.env.Define(node.Name.Value, genericConstructorType, false) {
		c.addError(node.Name, fmt.Sprintf("failed to define generic class '%s'", node.Name.Value))
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
func (c *Checker) createInstanceType(className string, body *parser.ClassBody, superClass *parser.Identifier, implements []*parser.Identifier) *types.ObjectType {
	// Create class instance type with metadata
	instanceType := types.NewClassInstanceType(className)

	// Handle inheritance relationships
	if superClass != nil {
		c.handleClassInheritance(instanceType, superClass.Value)
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
			getters[method.Key.Value] = method
		} else if method.Kind == "setter" && !method.IsStatic {
			setters[method.Key.Value] = method
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

			// Add method with access control metadata
			instanceType.WithClassMember(method.Key.Value, methodType, accessLevel, false, false)

			debugPrintf("// [Checker Class] Added method '%s' to instance type: %s (%s)\n",
				method.Key.Value, methodType.String(), accessLevel.String())
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

			debugPrintf("// [Checker Class] Validated method signature '%s'\n", methodSig.Key.Value)
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

// validateOverrideMethod validates the usage of the override keyword
func (c *Checker) validateOverrideMethod(method *parser.MethodDefinition, superClass *parser.Identifier, className string) {
	// Basic validation: if there's no superclass, override doesn't make sense
	if superClass == nil {
		c.addError(method.Key, fmt.Sprintf("method '%s' uses 'override' but class '%s' does not extend any class", method.Key.Value, className))
		return
	}

	// TODO: When inheritance is implemented, add:
	// 1. Check if the method exists in the superclass
	// 2. Check if the method signatures are compatible
	// 3. Check if the method is not final/sealed
	// 4. Check access modifier compatibility

	debugPrintf("// [Checker Class] Override validation for method '%s' in class '%s' (inheritance not yet implemented)\n",
		method.Key.Value, className)
}

// validateOverrideMethodSignature validates the usage of the override keyword for method signatures
func (c *Checker) validateOverrideMethodSignature(methodSig *parser.MethodSignature, superClass *parser.Identifier, className string) {
	// Basic validation: if there's no superclass, override doesn't make sense
	if superClass == nil {
		c.addError(methodSig.Key, fmt.Sprintf("method signature '%s' uses 'override' but class '%s' does not extend any class", methodSig.Key.Value, className))
		return
	}

	// TODO: When inheritance is implemented, add similar validation as for method definitions

	debugPrintf("// [Checker Class] Override validation for method signature '%s' in class '%s' (inheritance not yet implemented)\n",
		methodSig.Key.Value, className)
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
		debugPrintf("// [Checker Class] Method '%s' has no function value, using Any\n", method.Key.Value)
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

// handleClassInheritance processes class inheritance (extends clause)
func (c *Checker) handleClassInheritance(instanceType *types.ObjectType, superClassName string) {
	debugPrintf("// [Checker Class] Handling inheritance: %s extends %s\n", instanceType.GetClassName(), superClassName)

	// Check if superclass exists and is a class
	superType, _, exists := c.env.Resolve(superClassName)
	if !exists {
		c.addError(nil, fmt.Sprintf("superclass '%s' is not defined", superClassName))
		return
	}

	// Verify superType is a constructor (class)
	superObjType, ok := superType.(*types.ObjectType)
	if !ok {
		c.addError(nil, fmt.Sprintf("'%s' is not a class and cannot be extended", superClassName))
		return
	}

	// Check if it has constructor signatures (indicating it's a class constructor)
	if len(superObjType.ConstructSignatures) == 0 {
		c.addError(nil, fmt.Sprintf("'%s' is not a class and cannot be extended", superClassName))
		return
	}

	// Get the superclass instance type
	superInstanceType := c.getClassInstanceType(superClassName)
	if superInstanceType == nil {
		c.addError(nil, fmt.Sprintf("could not find instance type for superclass '%s'", superClassName))
		return
	}

	// Set inheritance relationship in class metadata
	if instanceType.ClassMeta != nil {
		instanceType.ClassMeta.SetSuperClass(superClassName)
	}

	// Add superclass to BaseTypes for structural inheritance
	instanceType.BaseTypes = append(instanceType.BaseTypes, superInstanceType)

	debugPrintf("// [Checker Class] Successfully set up inheritance: %s extends %s\n", instanceType.GetClassName(), superClassName)
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
			c.addError(sig.Key, fmt.Sprintf("invalid return type annotation for method '%s'", sig.Key.Value))
		}
	}
}

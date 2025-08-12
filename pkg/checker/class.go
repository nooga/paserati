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
		// Try to extract constant property name first
		if constantName, isConstant := c.tryExtractConstantComputedPropertyName(k.Expr); isConstant {
			return constantName
		}

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

// tryExtractConstantComputedPropertyName attempts to extract a constant property name
// from a computed property expression, returning the name and whether it's constant
func (c *Checker) tryExtractConstantComputedPropertyName(expr parser.Expression) (string, bool) {
	switch e := expr.(type) {
	case *parser.StringLiteral:
		return e.Value, true
	case *parser.NumberLiteral:
		return fmt.Sprintf("%v", e.Value), true
	case *parser.MemberExpression:
		// Check for Symbol.iterator
		if obj, ok := e.Object.(*parser.Identifier); ok && obj.Value == "Symbol" {
			if prop, ok := e.Property.(*parser.Identifier); ok && prop.Value == "iterator" {
				debugPrintf("// [Checker Class] Detected Symbol.iterator constant computed property\n")
				return "__COMPUTED_PROPERTY__", true
			}
		}
		debugPrintf("// [Checker Class] MemberExpression not Symbol.iterator: %T.%T\n", e.Object, e.Property)
		return "", false
	default:
		debugPrintf("// [Checker Class] Non-constant computed property expression: %T\n", expr)
		return "", false
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

		// Handle default type if present
		if param.DefaultType != nil {
			defaultType := c.resolveTypeAnnotation(param.DefaultType)
			if defaultType != nil {
				typeParam.Default = defaultType

				// Validate that default type satisfies constraint if both are present
				if typeParam.Constraint != nil && !types.IsAssignable(defaultType, typeParam.Constraint) {
					c.addError(param.DefaultType, fmt.Sprintf("default type '%s' does not satisfy constraint '%s'", defaultType.String(), typeParam.Constraint.String()))
				}
			}
		}

		typeParams[i] = typeParam
	}

	// 2. Create a new environment with type parameters available as TypeParameterType
	genericEnv := NewEnclosedEnvironment(c.env)
	for _, typeParam := range typeParams {
		// Define the type parameter properly in the environment
		if !genericEnv.DefineTypeParameter(typeParam.Name, typeParam) {
			c.addError(node.TypeParameters[0].Name, fmt.Sprintf("duplicate type parameter name: %s", typeParam.Name))
		}

		// Also make it available as a type alias for resolution
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

	// 3. Create a placeholder GenericType for the constructor to update the forward reference
	// This allows method bodies to reference the class constructor correctly
	placeholderConstructorType := &types.GenericType{
		Name:           node.Name.Value + "Constructor", // Internal name
		TypeParameters: typeParams,
		Body:           types.Any, // Placeholder, will be updated later
	}

	// 4. EARLY UPDATE: Replace the forward reference in the generic environment
	// This ensures that methods inside the class can access the constructor
	if !genericEnv.Update(node.Name.Value, placeholderConstructorType) {
		debugPrintf("// [Checker Class] Warning: Could not update generic environment for '%s'\n", node.Name.Value)
	} else {
		debugPrintf("// [Checker Class] Early updated generic environment for '%s' with placeholder constructor type\n", node.Name.Value)
	}

	// 5. Create the instance type body with TypeParameterType references
	instanceType := c.createInstanceType(node.Name.Value, node.Body, node.SuperClass, node.Implements)

	// 6. Create constructor signature from constructor method
	constructorSig := c.createConstructorSignature(node.Body, instanceType)

	// 7. Create constructor type
	constructorType := types.NewConstructorType(constructorSig)
	constructorType = c.addStaticMembers(node.Body, constructorType)

	// 8. Create the GenericType for the class
	genericClassType := &types.GenericType{
		Name:           node.Name.Value,
		TypeParameters: typeParams,
		Body:           instanceType,
	}

	// 9. Create the final GenericType for the constructor
	genericConstructorType := &types.GenericType{
		Name:           node.Name.Value + "Constructor", // Internal name
		TypeParameters: typeParams,
		Body:           constructorType,
	}

	// 10. UPDATE AGAIN: Replace the placeholder with the real constructor type
	if !genericEnv.Update(node.Name.Value, genericConstructorType) {
		debugPrintf("// [Checker Class] Warning: Could not update generic environment for '%s' with final type\n", node.Name.Value)
	} else {
		debugPrintf("// [Checker Class] Final updated generic environment for '%s' with real constructor type\n", node.Name.Value)
	}

	// Restore environment and current state
	c.env = savedEnv
	c.currentGenericClass = savedCurrentGenericClass
	c.currentForwardRef = savedCurrentForwardRef

	// 9. Update in main environment - store the generic constructor type (replacing forward reference)
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

	// Set current instance type for use in method type inference
	prevInstanceType := c.currentClassInstanceType
	c.currentClassInstanceType = instanceType
	defer func() { c.currentClassInstanceType = prevInstanceType }()

	// Handle inheritance relationships
	if superClass != nil {
		c.handleClassInheritance(instanceType, superClass)
	}

	// Handle interface implementations (registration only, validation deferred)
	var interfaceNames []string
	for _, iface := range implements {
		interfaceNames = append(interfaceNames, iface.Value)
		c.registerInterfaceImplementation(instanceType, iface.Value)
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
			// Add the property to the type
			instanceType.Properties[propName] = propType

			// Add specific getter/setter metadata
			if instanceType.ClassMeta != nil {
				if getter != nil && setter != nil {
					// Both getter and setter - for now, mark as getter (could be enhanced later)
					instanceType.ClassMeta.AddGetterMember(propName, accessLevel, false)
				} else if getter != nil {
					// Only getter
					instanceType.ClassMeta.AddGetterMember(propName, accessLevel, false)
				} else if setter != nil {
					// Only setter
					instanceType.ClassMeta.AddSetterMember(propName, accessLevel, false)
				}
			}

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
			if computedKey, isComputed := method.Key.(*parser.ComputedPropertyName); isComputed {
				// Try to extract constant property name
				if constantName, isConstant := c.tryExtractConstantComputedPropertyName(computedKey.Expr); isConstant {
					// This is a constant computed property like [Symbol.iterator] - add as specific method
					instanceType.WithClassMember(constantName, methodType, accessLevel, false, false)
					debugPrintf("// [Checker Class] Added constant computed method '%s' to instance type: %s (%s)\n",
						constantName, methodType.String(), accessLevel.String())
				} else {
					// This is a dynamic computed property - add to index signature
					hasComputedProperties = true
					computedValueTypes = append(computedValueTypes, methodType)
					debugPrintf("// [Checker Class] Found dynamic computed method, adding to index signature: %s\n", methodType.String())
				}
			} else {
				// Add method with access control metadata
				methodName := c.extractPropertyName(method.Key)
				instanceType.WithClassMember(methodName, methodType, accessLevel, false, false)

				debugPrintf("// [Checker Class] Added method '%s' to instance type: %s (%s)\n",
					methodName, methodType.String(), accessLevel.String())
			}
		}
	}

	// Process method signatures (overloads and abstract methods)
	for _, methodSig := range body.MethodSigs {
		if !methodSig.IsStatic {
			methodName := c.extractPropertyName(methodSig.Key)
			c.setClassContext(className, types.AccessContextInstanceMethod)

			// Validate override keyword usage for method signature
			if methodSig.IsOverride {
				c.validateOverrideMethodSignature(methodSig, superClass, className)
			}

			// Validate the signature types
			c.validateMethodSignature(methodSig)

			// For abstract methods, add them to the instance type
			if methodSig.IsAbstract {
				methodType := c.inferMethodTypeFromSignature(methodSig)
				accessLevel := c.getAccessLevel(methodSig.IsPublic, methodSig.IsPrivate, methodSig.IsProtected)

				instanceType.WithClassMember(methodName, methodType, accessLevel, false, false)
				debugPrintf("// [Checker Class] Added abstract method '%s' to instance type: %s\n", methodName, methodType.String())
			}

			debugPrintf("// [Checker Class] Processed method signature '%s'\n", methodName)
		}
	}

	// ADDED: Synthesize parameter properties from constructor before processing regular properties
	c.synthesizeParameterProperties(body)

	// Add properties to instance type (excluding static properties)
	for _, prop := range body.Properties {
		if !prop.IsStatic {
			propType := c.inferPropertyType(prop)

			// Determine access level
			accessLevel := c.getAccessLevel(prop.IsPublic, prop.IsPrivate, prop.IsProtected)

			// Check if this is a computed property
			if computedKey, isComputed := prop.Key.(*parser.ComputedPropertyName); isComputed {
				// Try to extract constant property name
				if constantName, isConstant := c.tryExtractConstantComputedPropertyName(computedKey.Expr); isConstant {
					// This is a constant computed property like [Symbol.iterator] - add as specific property
					instanceType.WithClassMember(constantName, propType, accessLevel, false, false)
					debugPrintf("// [Checker Class] Added constant computed property '%s' to instance type: %s (%s)\n",
						constantName, propType.String(), accessLevel.String())
				} else {
					// This is a dynamic computed property - add to index signature
					hasComputedProperties = true
					computedValueTypes = append(computedValueTypes, propType)
					debugPrintf("// [Checker Class] Found dynamic computed property, adding to index signature: %s\n", propType.String())
				}
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

	// FINAL PASS: Check method bodies now that all properties and methods are in the instance type
	c.checkMethodBodiesInInstance(className, body, instanceType)

	// VALIDATE INTERFACES: Now that all properties and methods are added, validate interface implementations
	for _, interfaceName := range interfaceNames {
		c.validateInterfaceImplementationDeferred(instanceType, interfaceName)
	}

	return instanceType
}

// checkMethodBodiesInInstance performs a final pass to check all method bodies
// after the complete instance type (with all properties) has been built
func (c *Checker) checkMethodBodiesInInstance(className string, body *parser.ClassBody, instanceType *types.ObjectType) {
	// Set up context for method body checking
	prevInstanceType := c.currentClassInstanceType
	c.currentClassInstanceType = instanceType
	defer func() { c.currentClassInstanceType = prevInstanceType }()

	// Set current this type to the instance type
	prevThisType := c.currentThisType
	c.currentThisType = instanceType
	defer func() { c.currentThisType = prevThisType }()

	debugPrintf("// [Checker Class] Final pass: checking method bodies for class '%s'\n", className)

	// Check all method bodies (including constructors, but excluding static and generic methods)
	for _, method := range body.Methods {
		if !method.IsStatic && method.Value != nil {
			// Skip generic methods - they should be checked when instantiated
			if len(method.Value.TypeParameters) > 0 {
				continue
			}

			methodName := c.extractPropertyName(method.Key)
			debugPrintf("// [Checker Class] Checking method body for '%s'\n", methodName)

			// Set appropriate context based on method kind
			if method.Kind == "constructor" {
				c.setClassContext(className, types.AccessContextConstructor)
			} else {
				c.setClassContext(className, types.AccessContextInstanceMethod)
			}

			// Check the method body - but preserve the context established here
			c.checkMethodBodyWithContext(method.Value)
		}
	}
}

// checkMethodBodyWithContext checks a method body while preserving class context
func (c *Checker) checkMethodBodyWithContext(fn *parser.FunctionLiteral) {
	// Save current context
	savedClassContext := c.currentClassContext
	savedThisType := c.currentThisType
	savedInstanceType := c.currentClassInstanceType
	savedEnv := c.env

	// Create a new environment scope for the method body
	c.env = NewEnclosedEnvironment(c.env)

	// Restore class context (it should already be set from the calling context)
	c.currentClassContext = savedClassContext
	c.currentThisType = savedThisType
	c.currentClassInstanceType = savedInstanceType

	// Set up 'super' in the environment if the class has a parent
	if savedInstanceType != nil && savedInstanceType.ClassMeta != nil && savedInstanceType.ClassMeta.SuperClassName != "" {
		// For constructor context, super should be the parent constructor
		// For method context, super should be the parent instance
		if savedClassContext != nil && savedClassContext.ContextType == types.AccessContextConstructor {
			// Get the parent constructor
			if superConstructor, _, exists := c.env.Resolve(savedInstanceType.ClassMeta.SuperClassName); exists {
				c.env.Define("super", superConstructor, true)
			}
		} else {
			// Get the parent instance type
			if superInstanceType := c.getClassInstanceType(savedInstanceType.ClassMeta.SuperClassName); superInstanceType != nil {
				c.env.Define("super", superInstanceType, true)
			}
		}
	}

	defer func() {
		// Restore environment and context after checking
		c.env = savedEnv
		c.currentClassContext = savedClassContext
		c.currentThisType = savedThisType
		c.currentClassInstanceType = savedInstanceType
	}()

	// Check the function literal
	c.checkFunctionLiteral(fn)
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

// inferMethodTypeFromSignature creates a function type from a method signature (for abstract methods)
func (c *Checker) inferMethodTypeFromSignature(methodSig *parser.MethodSignature) types.Type {
	// Extract parameter types
	var paramTypes []types.Type
	for _, param := range methodSig.Parameters {
		if param.TypeAnnotation != nil {
			paramType := c.resolveTypeAnnotation(param.TypeAnnotation)
			if paramType != nil {
				paramTypes = append(paramTypes, paramType)
			} else {
				paramTypes = append(paramTypes, types.Any)
			}
		} else {
			paramTypes = append(paramTypes, types.Any)
		}
	}

	// Extract return type
	var returnType types.Type = types.Void
	if methodSig.ReturnTypeAnnotation != nil {
		resolvedReturnType := c.resolveTypeAnnotation(methodSig.ReturnTypeAnnotation)
		if resolvedReturnType != nil {
			returnType = resolvedReturnType
		}
	}

	// Extract optional parameters
	optionalParams := make([]bool, len(methodSig.Parameters))
	for i, param := range methodSig.Parameters {
		optionalParams[i] = param.Optional
	}

	// Extract rest parameter type
	var restParameterType types.Type
	if methodSig.RestParameter != nil && methodSig.RestParameter.TypeAnnotation != nil {
		restType := c.resolveTypeAnnotation(methodSig.RestParameter.TypeAnnotation)
		if restType != nil {
			restParameterType = restType
		} else {
			restParameterType = &types.ArrayType{ElementType: types.Any}
		}
	}

	signature := &types.Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    optionalParams,
		IsVariadic:        methodSig.RestParameter != nil,
		RestParameterType: restParameterType,
	}

	return types.NewFunctionType(signature)
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

// isEmptyFunctionBody checks if a function body is empty (abstract methods)
func (c *Checker) isEmptyFunctionBody(body parser.Node) bool {
	if blockStmt, ok := body.(*parser.BlockStatement); ok {
		return len(blockStmt.Statements) == 0
	}
	return false
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

	// NOTE: Non-generic method body checking deferred to second pass after properties are added to instance type

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
			// Resolve default type if present
			var defaultType types.Type
			if typeParamNode.DefaultType != nil {
				defaultType = c.resolveTypeAnnotation(typeParamNode.DefaultType)
			}

			// Create the type parameter
			typeParam := &types.TypeParameter{
				Name:       typeParamNode.Name.Value,
				Constraint: types.Any, // Simple constraint for now
				Default:    defaultType,
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
		// A parameter is optional if it's explicitly marked optional (y?: number)
		// or if it has a default value (y: number = 42)
		optionalParams[i] = param.Optional || param.DefaultValue != nil
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
				// Resolve default type if present
				var defaultType types.Type
				if typeParamNode.DefaultType != nil {
					defaultType = c.resolveTypeAnnotation(typeParamNode.DefaultType)
				}

				// Create the type parameter
				typeParam := &types.TypeParameter{
					Name:       typeParamNode.Name.Value,
					Constraint: types.Any, // Simple constraint for now
					Default:    defaultType,
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

		// Properly instantiate the generic constructor with type arguments
		if len(genRef.TypeArguments) > 0 {
			debugPrintf("// [Checker Class] Instantiating generic base constructor with %d type args\n", len(genRef.TypeArguments))

			// Resolve type arguments to types
			resolvedTypeArgs := make([]types.Type, len(genRef.TypeArguments))
			for i, typeArg := range genRef.TypeArguments {
				resolvedTypeArgs[i] = c.resolveTypeAnnotation(typeArg)
				debugPrintf("// [Checker Class] Resolved type arg %d: %s\n", i, resolvedTypeArgs[i].String())
			}

			instantiatedConstructor := c.instantiateGenericType(baseConstructor.(*types.GenericType), resolvedTypeArgs, nil)
			constructorType = instantiatedConstructor
		} else {
			// No type arguments provided, use base constructor as-is
			constructorType = baseConstructor
		}
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
		// Store both the name and the resolved constructor type for efficient super call resolution
		instanceType.ClassMeta.SetSuperClassWithConstructor(superClassName, constructorType)
	}

	// Add superclass to BaseTypes for structural inheritance
	instanceType.BaseTypes = append(instanceType.BaseTypes, superInstanceType)

	debugPrintf("// [Checker Class] Successfully set up inheritance: %s extends %s\n", instanceType.GetClassName(), superClassExpr.String())
}

// registerInterfaceImplementation registers that a class implements an interface (without validation)
func (c *Checker) registerInterfaceImplementation(instanceType *types.ObjectType, interfaceName string) {
	debugPrintf("// [Checker Class] Registering interface implementation: %s implements %s\n", instanceType.GetClassName(), interfaceName)

	// Check if interface exists
	interfaceType, exists := c.env.ResolveType(interfaceName)
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

	debugPrintf("// [Checker Class] Successfully registered interface implementation: %s implements %s\n", instanceType.GetClassName(), interfaceName)
}

// validateInterfaceImplementationDeferred validates interface implementation after class is fully built
func (c *Checker) validateInterfaceImplementationDeferred(instanceType *types.ObjectType, interfaceName string) {
	debugPrintf("// [Checker Class] Validating deferred interface implementation: %s implements %s\n", instanceType.GetClassName(), interfaceName)

	// Check if interface exists (should exist since we registered it earlier)
	interfaceType, exists := c.env.ResolveType(interfaceName)
	if !exists {
		c.addError(nil, fmt.Sprintf("interface '%s' is not defined", interfaceName))
		return
	}

	// Verify interfaceType is an interface
	interfaceObjType, ok := interfaceType.(*types.ObjectType)
	if !ok {
		c.addError(nil, fmt.Sprintf("'%s' is not an interface and cannot be implemented", interfaceName))
		return
	}

	// Now validate that the class actually implements the interface
	c.validateInterfaceImplementation(instanceType, interfaceObjType, interfaceName)

	debugPrintf("// [Checker Class] Successfully validated interface implementation: %s implements %s\n", instanceType.GetClassName(), interfaceName)
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

	// Get the current class metadata - use currentClassInstanceType from the context
	// instead of looking it up by name to avoid timing issues
	classInstanceType := c.currentClassInstanceType
	if classInstanceType == nil || classInstanceType.ClassMeta == nil {
		c.addError(node, "super expression can only be used within a class context")
		node.SetComputedType(types.Any)
		return
	}

	// Check if the current class has a superclass
	superClassName := classInstanceType.ClassMeta.SuperClassName
	if superClassName == "" {
		c.addError(node, fmt.Sprintf("class '%s' does not extend any class", classInstanceType.GetClassName()))
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
				// Resolve default type if present
				var defaultType types.Type
				if typeParamNode.DefaultType != nil {
					defaultType = c.resolveTypeAnnotation(typeParamNode.DefaultType)
				}

				// Create the type parameter
				typeParam := &types.TypeParameter{
					Name:       typeParamNode.Name.Value,
					Constraint: types.Any, // Simple constraint for now
					Default:    defaultType,
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

// synthesizeParameterProperties creates PropertyDefinition nodes for constructor parameter properties
// This allows parameter properties to be processed by the regular property checking logic
func (c *Checker) synthesizeParameterProperties(body *parser.ClassBody) {
	// Find the constructor method
	var constructor *parser.MethodDefinition
	for _, method := range body.Methods {
		if method.Kind == "constructor" {
			constructor = method
			break
		}
	}

	if constructor == nil || constructor.Value == nil {
		return // No constructor or no function literal
	}

	// Process constructor parameters
	for _, param := range constructor.Value.Parameters {
		// Check if this parameter has property modifiers
		if param.IsPublic || param.IsPrivate || param.IsProtected || param.IsReadonly {
			// Create a synthetic PropertyDefinition for this parameter property
			propDef := &parser.PropertyDefinition{
				Token:          param.Token,
				Key:            param.Name,           // Use parameter name as property key
				Value:          nil,                  // Parameter properties have no initializer
				TypeAnnotation: param.TypeAnnotation, // Use parameter's type annotation
				Optional:       param.Optional,       // Parameter properties inherit optional status
				IsStatic:       false,                // Parameter properties are always instance properties
				Readonly:       param.IsReadonly,
				IsPublic:       param.IsPublic,
				IsPrivate:      param.IsPrivate,
				IsProtected:    param.IsProtected,
			}

			// Add this synthetic property to the class body
			body.Properties = append(body.Properties, propDef)

			debugPrintf("// [Checker Class] Synthesized parameter property '%s' (public: %t, private: %t, protected: %t, readonly: %t)\n",
				param.Name.Value, param.IsPublic, param.IsPrivate, param.IsProtected, param.IsReadonly)
		}
	}
}

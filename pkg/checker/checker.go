package checker

import (
	"fmt"
	"math/big"
	"os"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/errors"  // Added import
	"github.com/nooga/paserati/pkg/modules" // Added for real ModuleLoader integration
	"github.com/nooga/paserati/pkg/source"  // Added import for source context

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

const checkerDebug = false

func debugPrintf(format string, args ...interface{}) {
	if checkerDebug {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format, args...)
	}
}

// getExportSpecName extracts the name string from an export specifier's Local or Exported field
// which can be either an Identifier or StringLiteral (ES2022 string export names)
func getExportSpecName(expr parser.Expression) string {
	if expr == nil {
		return ""
	}
	if ident, ok := expr.(*parser.Identifier); ok {
		return ident.Value
	}
	if strLit, ok := expr.(*parser.StringLiteral); ok {
		return strLit.Value
	}
	return ""
}

// isStringConcatenatable checks if a type can be coerced to string in concatenation
// TypeScript allows concatenation with string for most types
func (c *Checker) isStringConcatenatable(t types.Type) bool {
	if t == nil {
		return false
	}

	switch t {
	case types.String, types.Number, types.Boolean, types.BigInt:
		return true
	case types.Null, types.Undefined, types.Void:
		return true // These convert to strings in JS
	case types.Any:
		return true
	}

	// Handle union types - all members must be string-concatenatable
	if unionType, ok := t.(*types.UnionType); ok {
		for _, member := range unionType.Types {
			if !c.isStringConcatenatable(member) {
				return false
			}
		}
		return true
	}

	// Handle literal types
	if _, ok := t.(*types.LiteralType); ok {
		// Number and string literals are always concatenatable
		return true
	}

	// Handle enum member types
	if types.IsEnumMemberType(t) {
		return true // Enum members can be coerced to strings
	}

	// Be conservative for other types
	return false
}

// extractTypeParametersFromSignature extracts type parameter instances from a function signature
// This helps maintain consistency between hoisting and body checking phases
func (c *Checker) extractTypeParametersFromSignature(sig *types.Signature) []*types.TypeParameter {
	var typeParams []*types.TypeParameter
	seen := make(map[*types.TypeParameter]bool)

	// Helper function to extract type parameters from a type
	var extractFromType func(t types.Type)
	extractFromType = func(t types.Type) {
		switch typ := t.(type) {
		case *types.TypeParameterType:
			if !seen[typ.Parameter] {
				typeParams = append(typeParams, typ.Parameter)
				seen[typ.Parameter] = true
			}
		case *types.ArrayType:
			extractFromType(typ.ElementType)
		case *types.UnionType:
			for _, memberType := range typ.Types {
				extractFromType(memberType)
			}
		case *types.ObjectType:
			for _, propType := range typ.Properties {
				extractFromType(propType)
			}
		case *types.MappedType:
			// Extract from constraint type (e.g., K in { [P in K]: V })
			if typ.ConstraintType != nil {
				extractFromType(typ.ConstraintType)
			}
			// Extract from value type (e.g., V in { [P in K]: V })
			if typ.ValueType != nil {
				extractFromType(typ.ValueType)
			}
		case *types.TupleType:
			for _, elemType := range typ.ElementTypes {
				extractFromType(elemType)
			}
			if typ.RestElementType != nil {
				extractFromType(typ.RestElementType)
			}
		}
	}

	// Extract from parameter types
	for _, paramType := range sig.ParameterTypes {
		extractFromType(paramType)
	}

	// Extract from return type
	if sig.ReturnType != nil {
		extractFromType(sig.ReturnType)
	}

	// Extract from rest parameter type
	if sig.RestParameterType != nil {
		extractFromType(sig.RestParameterType)
	}

	return typeParams
}

// resolveParameterizedForwardReference attempts to resolve a ParameterizedForwardReferenceType
// to its actual instantiated generic type
func (c *Checker) resolveParameterizedForwardReference(paramRef *types.ParameterizedForwardReferenceType) types.Type {
	debugPrintf("// [Checker ResolveParamRef] Attempting to resolve %s\n", paramRef.String())

	// For generic classes, during method body checking, we can access the current forward reference
	// to get the instance type directly without going through the constructor
	if c.currentForwardRef != nil && c.currentForwardRef.ClassName == paramRef.ClassName {
		debugPrintf("// [Checker ResolveParamRef] Using current forward reference for %s\n", paramRef.ClassName)

		// If we're inside the same generic class, we need to create an instantiated version
		// of the current class's instance type with the type arguments from the paramRef
		if c.currentClassInstanceType != nil {
			debugPrintf("// [Checker ResolveParamRef] Found current class instance type: %s\n", c.currentClassInstanceType.String())

			// Create a substitution map from type parameters to the provided type arguments
			if len(paramRef.TypeArguments) == len(c.currentForwardRef.TypeParameters) {
				substitution := make(map[string]types.Type)
				for i, typeParam := range c.currentForwardRef.TypeParameters {
					substitution[typeParam.Name] = paramRef.TypeArguments[i]
					debugPrintf("// [Checker ResolveParamRef] Mapping %s -> %s\n", typeParam.Name, paramRef.TypeArguments[i].String())
				}

				// Substitute the type parameters in the instance type
				resolvedType := c.substituteTypes(c.currentClassInstanceType, substitution)
				debugPrintf("// [Checker ResolveParamRef] Substituted instance type: %s\n", resolvedType.String())
				return resolvedType
			}
		}
	}

	// Look up the generic type by class name
	if typ, _, found := c.env.Resolve(paramRef.ClassName); found {
		debugPrintf("// [Checker ResolveParamRef] Found type for %s: %T\n", paramRef.ClassName, typ)

		// Check if it's a generic type
		if genericType, ok := typ.(*types.GenericType); ok {
			debugPrintf("// [Checker ResolveParamRef] It's a generic type, instantiating with %d type args\n", len(paramRef.TypeArguments))

			// Instantiate the generic type with the provided type arguments
			instantiated := c.instantiateGenericType(genericType, paramRef.TypeArguments, nil)

			// If the instantiation resulted in an ObjectType with the instance type, extract it
			if objType, ok := instantiated.(*types.ObjectType); ok && len(objType.ConstructSignatures) > 0 {
				// Return the instance type (return type of the constructor)
				return objType.ConstructSignatures[0].ReturnType
			}

			return instantiated
		}
	}

	debugPrintf("// [Checker ResolveParamRef] Could not resolve %s\n", paramRef.ClassName)
	return nil // Could not resolve
}

// ContextualType represents type information that flows from context to sub-expressions
type ContextualType struct {
	ExpectedType types.Type // The type expected in this context
	IsContextual bool       // Whether this is a contextual hint vs. required type
}

// Checker performs static type checking on the AST.
type Checker struct {
	program *parser.Program    // Root AST node
	source  *source.SourceFile // Source context for error reporting (cached from program)
	// TODO: Add Type Registry if needed
	env    *Environment           // Current type environment
	errors []errors.PaseratiError // Changed from []TypeError

	// --- Module System Integration ---
	moduleEnv    *ModuleEnvironment   // Module-aware environment (nil for non-module files)
	moduleLoader modules.ModuleLoader // Real ModuleLoader for dependency resolution

	// --- NEW: Context for checking function bodies ---
	// Expected return type of the function currently being checked (set by explicit annotation).
	currentExpectedReturnType types.Type
	// List of types found in return statements within the current function (used for inference).
	currentInferredReturnTypes []types.Type
	// List of types found in yield expressions within the current generator function (used for inference).
	currentInferredYieldTypes []types.Type

	// --- NEW: Context for 'this' type checking ---
	// Type of 'this' in the current context (set when checking methods)
	currentThisType types.Type

	// --- NEW: Context for access control checking ---
	// Current class context for access modifier validation
	currentClassContext *types.AccessContext

	// --- NEW: Current class instance type being built ---
	// Used to set proper 'this' context in class methods
	currentClassInstanceType *types.ObjectType

	// --- NEW: Abstract classes tracking ---
	// Track abstract class names to prevent instantiation
	abstractClasses map[string]bool

	// --- NEW: Current generic class tracking ---
	// Track the current generic class being processed to allow forward references
	currentGenericClass *types.GenericType
	currentForwardRef   *types.ForwardReferenceType

	// --- NEW: Lenient mode for forward references ---
	// When true, undefined variables are treated as 'any' instead of errors
	// This allows checking method bodies that reference variables declared later
	allowForwardReferences bool

	// --- NEW: Generator function tracking ---
	// Track generator function names for yield* validation
	generatorFunctions map[string]bool

	// --- NEW: Current function context ---
	// Track if we're currently inside an async or generator function
	inAsyncFunction      bool
	inGeneratorFunction  bool
	functionNestingDepth int // 0 = top level, >0 = inside function(s)

	// --- NEW: Object method context ---
	// Track if we're currently inside an object method (for super support)
	inObjectMethod bool

	// --- NEW: Eval super context ---
	// Track if super is allowed in the current eval context (eval called from method)
	allowSuperInEval bool

	// --- NEW: Recursive type alias tracking ---
	// Track type aliases being resolved to prevent infinite recursion
	resolvingTypeAliases map[string]bool

	// --- NEW: Anonymous class tracking ---
	// Counter for generating unique anonymous class names
	anonymousClassCounter int
}

// NewChecker creates a new type checker with standard built-in types.
func NewChecker() *Checker {
	return NewCheckerWithInitializers(builtins.GetStandardInitializers())
}

// NewCheckerWithInitializers creates a new type checker with custom built-in initializers.
func NewCheckerWithInitializers(initializers []builtins.BuiltinInitializer) *Checker {
	return &Checker{
		env:    NewGlobalEnvironment(initializers), // Create persistent global environment with custom initializers
		errors: []errors.PaseratiError{},           // Initialize with correct type
		// Initialize function context fields to nil/empty
		currentExpectedReturnType:  nil,
		currentInferredReturnTypes: nil,
		currentThisType:            nil, // Initialize this type context
		currentClassContext:        nil, // No class context initially
		abstractClasses:            make(map[string]bool),
		generatorFunctions:         make(map[string]bool),
		resolvingTypeAliases:       make(map[string]bool),
	}
}

// GetEnvironment returns the current type environment
func (c *Checker) GetEnvironment() *Environment {
	return c.env
}

// SetAllowSuperInEval sets whether super expressions are allowed in eval contexts
// This is used when compiling direct eval code that was called from a method context
func (c *Checker) SetAllowSuperInEval(allow bool) {
	c.allowSuperInEval = allow
}

// --- Access Control Helper Methods ---

// setClassContext sets the current class context for access control checking
func (c *Checker) setClassContext(className string, contextType types.AccessContextType) {
	c.currentClassContext = types.NewAccessContext(className, contextType)
	// Set inheritance checking function
	c.currentClassContext.IsSubclassOfFunc = c.isSubclassOf
}

// clearClassContext clears the current class context
func (c *Checker) clearClassContext() {
	c.currentClassContext = nil
}

// isSubclassOf checks if currentClass is a subclass of targetClass
func (c *Checker) isSubclassOf(currentClass, targetClass string) bool {
	if currentClass == targetClass {
		return false // Same class is not a subclass of itself
	}

	// Look up the current class type - try type alias first
	currentType, exists := c.env.ResolveType(currentClass)
	if !exists {
		// Fall back to variable lookup
		currentType, _, exists = c.env.Resolve(currentClass)
		if !exists {
			return false
		}
	}

	// Get the class metadata
	var currentClassType *types.ObjectType

	// Handle both direct class types and constructor types
	if forwardRef, ok := currentType.(*types.ForwardReferenceType); ok {
		// For forward references, use the current class instance type context
		if c.currentClassInstanceType != nil && c.currentClassInstanceType.GetClassName() == forwardRef.ClassName {
			currentClassType = c.currentClassInstanceType
		}
	} else if objectType, ok := currentType.(*types.ObjectType); ok {
		// If it's a constructor (has construct signatures), get the instance type
		if len(objectType.ConstructSignatures) > 0 && objectType.ConstructSignatures[0].ReturnType != nil {
			if instanceType, ok := objectType.ConstructSignatures[0].ReturnType.(*types.ObjectType); ok {
				currentClassType = instanceType
			}
		} else if objectType.IsClassInstance() {
			// If it's already an instance type
			currentClassType = objectType
		}
	} else if genericType, ok := currentType.(*types.GenericType); ok {
		if constructorType, ok := genericType.Body.(*types.ObjectType); ok {
			if len(constructorType.ConstructSignatures) > 0 && constructorType.ConstructSignatures[0].ReturnType != nil {
				if instanceType, ok := constructorType.ConstructSignatures[0].ReturnType.(*types.ObjectType); ok {
					currentClassType = instanceType
				}
			}
		}
	}

	if currentClassType == nil || !currentClassType.IsClassInstance() {
		return false
	}

	// Check inheritance chain
	return c.checkInheritanceChain(currentClassType, targetClass)
}

// checkInheritanceChain recursively checks if a class inherits from targetClass
func (c *Checker) checkInheritanceChain(classType *types.ObjectType, targetClass string) bool {
	// Check direct parent
	if classType.BaseTypes != nil {
		for _, baseType := range classType.BaseTypes {
			if baseObj, ok := baseType.(*types.ObjectType); ok {
				baseClassName := baseObj.GetClassName()
				// Check if this base class matches target
				if baseClassName == targetClass {
					return true
				}
				// Recursively check base class inheritance
				if c.checkInheritanceChain(baseObj, targetClass) {
					return true
				}
			}
		}
	}
	return false
}

// validateMemberAccess checks if a member access is allowed given the current context
func (c *Checker) validateMemberAccess(objectType types.Type, memberName string, accessLocation parser.Node) {
	// Only check access for class instance types
	if objType, ok := objectType.(*types.ObjectType); ok && objType.IsClassInstance() {
		// Check if member is accessible from current context
		if !objType.IsAccessibleFrom(memberName, c.currentClassContext) {
			c.createAccessError(objType, memberName, accessLocation)
		}
	}
}

// createAccessError creates a TypeScript-style access control error
func (c *Checker) createAccessError(objType *types.ObjectType, memberName string, node parser.Node) {
	memberInfo := objType.GetMemberAccessInfo(memberName)
	className := objType.GetClassName()

	var errorMsg string
	if memberInfo != nil {
		errorMsg = fmt.Sprintf("Property '%s' is %s and only accessible within class '%s'",
			memberName, memberInfo.AccessLevel.String(), className)
	} else {
		errorMsg = fmt.Sprintf("Property '%s' does not exist on type '%s'", memberName, className)
	}

	c.addError(node, errorMsg)
}

// getCurrentClassName returns the current class name being checked, or empty string
func (c *Checker) getCurrentClassName() string {
	if c.currentClassContext != nil {
		return c.currentClassContext.CurrentClassName
	}
	return ""
}

// Check analyzes the given program AST for type errors.
func (c *Checker) Check(program *parser.Program) []errors.PaseratiError {
	c.program = program
	c.source = program.Source           // Cache source for error reporting
	c.errors = []errors.PaseratiError{} // Reset errors
	// DON'T reset the environment - keep it persistent for REPL sessions
	// c.env = NewGlobalEnvironment()      // Start with a fresh global environment for this check
	globalEnv := c.env

	// --- Data Structures for Passes ---
	nodesProcessedPass1 := make(map[parser.Node]bool)   // Nodes handled in Pass 1 (Type Aliases)
	nodesProcessedPass2 := make(map[parser.Node]bool)   // Nodes handled in Pass 2 (Signatures/Vars)
	functionsToVisitBody := []*parser.FunctionLiteral{} // Function literals needing body check in Pass 3

	// --- Pass 0: Pre-register all interface names as forward references ---
	// This allows mutually recursive interfaces like:
	//   interface A { b: B; }
	//   interface B { a: A; }
	// NOTE: Only pre-register NON-GENERIC interfaces and type aliases. Generic types
	// are handled differently because they need proper type parameter processing.
	debugPrintf("\n// --- Checker - Pass 0: Pre-registering Interface and Type Alias Names ---\n")
	for _, stmt := range program.Statements {
		if interfaceStmt, ok := stmt.(*parser.InterfaceDeclaration); ok {
			// Only pre-register non-generic interfaces
			if len(interfaceStmt.TypeParameters) == 0 {
				if _, exists := c.env.ResolveType(interfaceStmt.Name.Value); !exists {
					placeholderType := &types.ObjectType{
						Properties:         make(map[string]types.Type),
						OptionalProperties: make(map[string]bool),
					}
					c.env.DefineTypeAlias(interfaceStmt.Name.Value, placeholderType)
					debugPrintf("// [Checker Pass 0] Pre-registered interface '%s'\n", interfaceStmt.Name.Value)
				}
			}
		} else if aliasStmt, ok := stmt.(*parser.TypeAliasStatement); ok {
			// Only pre-register non-generic type aliases
			if len(aliasStmt.TypeParameters) == 0 {
				if _, exists := c.env.ResolveType(aliasStmt.Name.Value); !exists {
					// Use a forward reference placeholder that will be resolved in Pass 1
					placeholderType := &types.TypeAliasForwardReference{
						AliasName: aliasStmt.Name.Value,
					}
					c.env.DefineTypeAlias(aliasStmt.Name.Value, placeholderType)
					debugPrintf("// [Checker Pass 0] Pre-registered type alias '%s'\n", aliasStmt.Name.Value)
				}
			}
		} else if exportStmt, ok := stmt.(*parser.ExportNamedDeclaration); ok {
			if exportStmt.Declaration != nil {
				if interfaceStmt, ok := exportStmt.Declaration.(*parser.InterfaceDeclaration); ok {
					// Only pre-register non-generic interfaces
					if len(interfaceStmt.TypeParameters) == 0 {
						if _, exists := c.env.ResolveType(interfaceStmt.Name.Value); !exists {
							placeholderType := &types.ObjectType{
								Properties:         make(map[string]types.Type),
								OptionalProperties: make(map[string]bool),
							}
							c.env.DefineTypeAlias(interfaceStmt.Name.Value, placeholderType)
							debugPrintf("// [Checker Pass 0] Pre-registered exported interface '%s'\n", interfaceStmt.Name.Value)
						}
					}
				} else if aliasStmt, ok := exportStmt.Declaration.(*parser.TypeAliasStatement); ok {
					// Only pre-register non-generic type aliases
					if len(aliasStmt.TypeParameters) == 0 {
						if _, exists := c.env.ResolveType(aliasStmt.Name.Value); !exists {
							placeholderType := &types.TypeAliasForwardReference{
								AliasName: aliasStmt.Name.Value,
							}
							c.env.DefineTypeAlias(aliasStmt.Name.Value, placeholderType)
							debugPrintf("// [Checker Pass 0] Pre-registered exported type alias '%s'\n", aliasStmt.Name.Value)
						}
					}
				}
			}
		}
	}

	// --- Pass 1: Define ALL Type Aliases ---
	debugPrintf("\n// --- Checker - Pass 1: Defining Type Aliases, Interfaces, Classes, and Processing Imports ---\n")
	for _, stmt := range program.Statements {
		debugPrintf("// [Checker Pass 1] Examining statement type: %T\n", stmt)
		if aliasStmt, ok := stmt.(*parser.TypeAliasStatement); ok {
			debugPrintf("// [Checker Pass 1] Processing Type Alias: %s\n", aliasStmt.Name.Value)
			c.checkTypeAliasStatement(aliasStmt) // Uses c.env (globalEnv)
			nodesProcessedPass1[aliasStmt] = true
			nodesProcessedPass2[aliasStmt] = true // Also mark for Pass 2 skip
		} else if interfaceStmt, ok := stmt.(*parser.InterfaceDeclaration); ok {
			debugPrintf("// [Checker Pass 1] Processing Interface: %s\n", interfaceStmt.Name.Value)
			c.checkInterfaceDeclaration(interfaceStmt)
			nodesProcessedPass1[interfaceStmt] = true
			nodesProcessedPass2[interfaceStmt] = true // Also mark for Pass 2 skip
		} else if classStmt, ok := stmt.(*parser.ClassDeclaration); ok {
			debugPrintf("// [Checker Pass 1] Processing Class: %s\n", classStmt.Name.Value)
			c.checkClassDeclaration(classStmt)
			nodesProcessedPass1[classStmt] = true
			nodesProcessedPass2[classStmt] = true // Also mark for Pass 2 skip
		} else if importStmt, ok := stmt.(*parser.ImportDeclaration); ok {
			// Add defensive check for nil Source
			if importStmt.Source != nil {
				debugPrintf("// [Checker Pass 1] Processing Import: %s\n", importStmt.Source.Value)
				c.checkImportDeclaration(importStmt)
			} else {
				debugPrintf("// [Checker Pass 1] Skipping Import with nil Source\n")
			}
			nodesProcessedPass1[importStmt] = true
			nodesProcessedPass2[importStmt] = true // Also mark for Pass 2 skip
		} else if exportStmt, ok := stmt.(*parser.ExportNamedDeclaration); ok {
			// Handle exported type declarations (interfaces, type aliases, classes)
			if exportStmt.Declaration != nil {
				if interfaceStmt, ok := exportStmt.Declaration.(*parser.InterfaceDeclaration); ok {
					debugPrintf("// [Checker Pass 1] Processing Exported Interface: %s\n", interfaceStmt.Name.Value)
					c.checkInterfaceDeclaration(interfaceStmt)
					nodesProcessedPass1[interfaceStmt] = true
					nodesProcessedPass2[interfaceStmt] = true // Also mark for Pass 2 skip
				} else if aliasStmt, ok := exportStmt.Declaration.(*parser.TypeAliasStatement); ok {
					debugPrintf("// [Checker Pass 1] Processing Exported Type Alias: %s\n", aliasStmt.Name.Value)
					c.checkTypeAliasStatement(aliasStmt)
					nodesProcessedPass1[aliasStmt] = true
					nodesProcessedPass2[aliasStmt] = true // Also mark for Pass 2 skip
				} else if classStmt, ok := exportStmt.Declaration.(*parser.ClassDeclaration); ok {
					debugPrintf("// [Checker Pass 1] Processing Exported Class: %s\n", classStmt.Name.Value)
					c.checkClassDeclaration(classStmt)
					nodesProcessedPass1[classStmt] = true
					nodesProcessedPass2[classStmt] = true // Also mark for Pass 2 skip
				}
			}
		} else if exprStmt, ok := stmt.(*parser.ExpressionStatement); ok {
			// Check if this is a class or enum expression wrapped in an expression statement
			if enumDecl, isEnumDecl := exprStmt.Expression.(*parser.EnumDeclaration); isEnumDecl && enumDecl.Name != nil {
				debugPrintf("// [Checker Pass 1] Processing Enum Declaration: %s\n", enumDecl.Name.Value)
				c.checkEnumDeclaration(enumDecl)
				nodesProcessedPass1[exprStmt] = true
				nodesProcessedPass2[exprStmt] = true // Also mark for Pass 2 skip
			} else if classExpr, isClassExpr := exprStmt.Expression.(*parser.ClassExpression); isClassExpr && classExpr.Name != nil {
				debugPrintf("// [Checker Pass 1] Processing Class Expression: %s\n", classExpr.Name.Value)
				// Convert to ClassDeclaration for checking
				classDecl := &parser.ClassDeclaration{
					Token:          classExpr.Token,
					Name:           classExpr.Name,
					TypeParameters: classExpr.TypeParameters,
					SuperClass:     classExpr.SuperClass,
					Body:           classExpr.Body,
					IsAbstract:     classExpr.IsAbstract,
				}
				c.checkClassDeclaration(classDecl)
				nodesProcessedPass1[exprStmt] = true
				nodesProcessedPass2[exprStmt] = true // Also mark for Pass 2 skip
			}
		}
	}
	debugPrintf("// --- Checker - Pass 1: Complete ---\n")

	// --- Pass 2: Initial Declarations (Variables & Function Signatures) ---
	debugPrintf("\n// --- Checker - Pass 2: Initial Declarations (Shallow) ---\n")
	c.env = globalEnv // Ensure we are in the global scope

	// First, handle hoisted functions explicitly
	if program.HoistedDeclarations != nil {
		for name, hoistedNode := range program.HoistedDeclarations {
			funcLit, _ := hoistedNode.(*parser.FunctionLiteral) // Parser guarantees type

			debugPrintf("// [Checker Pass 2] Processing Hoisted Function Signature: %s\n", name)

			// Use the new unified approach for generic function support
			ctx := &FunctionCheckContext{
				FunctionName:            name,
				TypeParameters:          funcLit.TypeParameters, // Support for generic functions
				Parameters:              funcLit.Parameters,
				RestParameter:           funcLit.RestParameter,
				ReturnTypeAnnotation:    funcLit.ReturnTypeAnnotation,
				Body:                    nil, // Don't process body during hoisting
				IsArrow:                 false,
				IsGenerator:             funcLit.IsGenerator, // Detect generator functions during hoisting
				IsAsync:                 funcLit.IsAsync,     // Detect async functions during hoisting
				AllowSelfReference:      false,               // Don't allow self-reference during hoisting
				AllowOverloadCompletion: false,               // Don't check overloads during hoisting
			}

			// Resolve parameters and signature with type parameter support
			initialSignature, _, _, _, _, _ := c.resolveFunctionParameters(ctx)
			if initialSignature == nil { // Handle resolution error
				initialSignature = &types.Signature{ // Default to Any signature on error
					ParameterTypes: make([]types.Type, len(funcLit.Parameters)),
					ReturnType:     types.Any,
				}
				for i := range initialSignature.ParameterTypes {
					initialSignature.ParameterTypes[i] = types.Any
				}
			}

			// For generator functions, defer the Generator wrapping to detailed checking (Pass 3)
			// to ensure proper return type validation within the function body
			if funcLit.IsGenerator {
				debugPrintf("// [Checker Pass 2] Generator function %s - deferring Generator wrapping to Pass 3\n", name)
				// Track this as a generator function for yield* validation
				c.generatorFunctions[name] = true
				// Keep the inner return type for hoisting, Generator wrapping happens in Pass 3
			}

			// Convert signature to ObjectType for storage
			initialObjectType := types.NewFunctionType(initialSignature)
			if !globalEnv.Define(name, initialObjectType, false) { // Define with initial (maybe incomplete) signature
				c.addError(funcLit.Name, fmt.Sprintf("identifier '%s' already defined (hoisted)", name))
			}
			funcLit.SetComputedType(initialObjectType) // Set initial type on node
			functionsToVisitBody = append(functionsToVisitBody, funcLit)
			nodesProcessedPass2[funcLit] = true // Mark the FuncLit node itself

			// Also mark the wrapping ExpressionStatement (if any)
			for _, stmt := range program.Statements {
				if es, ok := stmt.(*parser.ExpressionStatement); ok && es.Expression == funcLit {
					nodesProcessedPass2[es] = true
					break
				}
			}
			debugPrintf("// [Checker Pass 2] Defined hoisted func '%s' with initial type: %s\n", name, initialSignature.String())
		}
	}

	// Now, iterate through remaining statements for variables and non-hoisted functions
	for _, stmt := range program.Statements {
		if nodesProcessedPass1[stmt] || nodesProcessedPass2[stmt] { // Skip if already handled
			continue
		}

		switch node := stmt.(type) {
		case *parser.ExpressionStatement:
			// --- NEW: Handle FunctionSignature expressions during Pass 2 ---
			if sigExpr, ok := node.Expression.(*parser.FunctionSignature); ok {
				debugPrintf("// [Checker Pass 2] Processing Function Signature: %s\n", sigExpr.Name.Value)
				c.processFunctionSignature(sigExpr)
				nodesProcessedPass2[stmt] = true // Mark as processed
				continue
			}
			// Skip other expression statements for now
			debugPrintf("// [Checker Pass 2] Skipping ExpressionStatement (not function signature)\n")

		case *parser.LetStatement, *parser.ConstStatement, *parser.VarStatement:
			// Process all declarations in the statement
			var declarations []*parser.VarDeclarator
			isConst := false
			stmtType := ""

			switch specificNode := node.(type) {
			case *parser.LetStatement:
				declarations = specificNode.Declarations
				stmtType = "Let"
			case *parser.ConstStatement:
				declarations = specificNode.Declarations
				isConst = true
				stmtType = "Const"
			case *parser.VarStatement:
				declarations = specificNode.Declarations
				stmtType = "Var"
			}

			for _, declarator := range declarations {
				varName := declarator.Name
				typeAnnotation := declarator.TypeAnnotation
				initializer := declarator.Value

				debugPrintf("// [Checker Pass 2] Processing %s: %s\n", stmtType, varName.Value)

				var declaredType types.Type
				if typeAnnotation != nil {
					declaredType = c.resolveTypeAnnotation(typeAnnotation) // Use globalEnv implicitly
				}

				var preliminaryType types.Type = declaredType // Start with annotation type

				// Check initializer specifically for FunctionLiteral
				if funcLitInitializer, ok := initializer.(*parser.FunctionLiteral); ok {
					debugPrintf("// [Checker Pass 2] Variable '%s' initialized with FunctionLiteral\n", varName.Value)
					initialFuncSignature := c.resolveFunctionLiteralSignature(funcLitInitializer, globalEnv)
					if initialFuncSignature == nil { // Handle resolution error
						initialFuncSignature = &types.Signature{ // Default to Any signature on error
							ParameterTypes: make([]types.Type, len(funcLitInitializer.Parameters)),
							ReturnType:     types.Any,
						}
						for i := range initialFuncSignature.ParameterTypes {
							initialFuncSignature.ParameterTypes[i] = types.Any
						}
					}
					// Convert signature to ObjectType and use function signature type if no annotation, or check compatibility if annotation exists
					initialFuncObjectType := types.NewFunctionType(initialFuncSignature)
					if preliminaryType == nil {
						preliminaryType = initialFuncObjectType
					} else {
						// TODO: Check if initialFuncObjectType is assignable to declaredType?
						// For now, declaredType takes precedence if both exist.
					}
					funcLitInitializer.SetComputedType(initialFuncObjectType) // Set initial type on the initializer node
					functionsToVisitBody = append(functionsToVisitBody, funcLitInitializer)
					nodesProcessedPass2[funcLitInitializer] = true // Mark initializer node if it's a func lit
					debugPrintf("// [Checker Pass 2] Added initializer func for '%s' to visit list\n", varName.Value)
				}

				// Fallback type if still nil
				if preliminaryType == nil {
					preliminaryType = types.Any // Or Undefined if no initializer? Let's use Any for now.
				}

				// Define variable in the environment
				if !globalEnv.Define(varName.Value, preliminaryType, isConst) {
					c.addError(varName, fmt.Sprintf("identifier '%s' already declared", varName.Value))
				} else {
					debugPrintf("// [Checker Pass 2] Defined var '%s' with initial type: %s\n", varName.Value, preliminaryType.String())
				}
				// Set type on the Name node itself
				varName.SetComputedType(preliminaryType)
			}
			nodesProcessedPass2[stmt] = true // Mark the Let/Const/Var statement itself

		default:
			// Skip other statement types (e.g., ExpressionStatement) in this pass
			debugPrintf("// [Checker Pass 2] Skipping statement type %T\n", node)
		}
	}
	debugPrintf("// --- Checker - Pass 2: Complete ---\n")

	// --- Pass 3: Function Body Analysis & Type Refinement ---
	debugPrintf("\n// --- Checker - Pass 3: Function Body Analysis ---\n")
	c.env = globalEnv // Ensure we start in the global scope for lookups inside functions
	for _, funcLit := range functionsToVisitBody {
		funcNameForLog := "<anonymous_or_assigned>"
		if funcLit.Name != nil {
			funcNameForLog = funcLit.Name.Value
		}
		debugPrintf("// [Checker Pass 3] Visiting body of func: %s (%p)\n", funcNameForLog, funcLit)

		// Get the initial ObjectType determined in Pass 2
		initialType := funcLit.GetComputedType()
		funcObjectType, ok := initialType.(*types.ObjectType)
		if !ok || funcObjectType == nil || !funcObjectType.IsCallable() {
			debugPrintf("// [Checker Pass 3] ERROR: Could not get initial ObjectType for %s\n", funcNameForLog)
			// Maybe try resolving again? Or skip? Let's skip for now.
			c.addError(funcLit, fmt.Sprintf("internal checker error: failed to retrieve initial signature for %s", funcNameForLog))
			continue
		}

		// Get the first call signature
		if len(funcObjectType.CallSignatures) == 0 {
			debugPrintf("// [Checker Pass 3] ERROR: ObjectType has no call signatures for %s\n", funcNameForLog)
			c.addError(funcLit, fmt.Sprintf("internal checker error: function has no call signatures for %s", funcNameForLog))
			continue
		}

		funcSignature := funcObjectType.CallSignatures[0]

		// Save outer context & Set context for body check
		outerExpectedReturnType := c.currentExpectedReturnType
		outerInferredReturnTypes := c.currentInferredReturnTypes
		outerInferredYieldTypes := c.currentInferredYieldTypes
		outerThisType := c.currentThisType
		outerInAsyncFunction := c.inAsyncFunction
		outerInGeneratorFunction := c.inGeneratorFunction

		c.currentExpectedReturnType = funcSignature.ReturnType // Use return type from initial signature
		c.currentInferredReturnTypes = nil
		c.currentInferredYieldTypes = []types.Type{} // Always collect yield types for generators
		c.inAsyncFunction = funcLit.IsAsync
		c.inGeneratorFunction = funcLit.IsGenerator
		c.functionNestingDepth++

		if c.currentExpectedReturnType == nil {
			c.currentInferredReturnTypes = []types.Type{} // Allocate only if inference needed
		}

		// Handle explicit 'this' parameter
		if len(funcLit.Parameters) > 0 && funcLit.Parameters[0].IsThis {
			// Set the 'this' context from the first parameter's type
			if len(funcSignature.ParameterTypes) > 0 {
				c.currentThisType = funcSignature.ParameterTypes[0]
				debugPrintf("// [Checker Pass 3] Setting this type from explicit parameter: %s\n", c.currentThisType.String())
			} else {
				// Should not happen if resolution worked correctly
				c.currentThisType = types.Any
			}
		} else {
			// No explicit 'this' parameter - set to 'any' for top-level functions
			c.currentThisType = types.Any
			debugPrintf("// [Checker Pass 3] No explicit this parameter, setting this to any for potential constructor function\n")
		}

		// Create function's inner scope & define parameters
		originalEnv := c.env

		// Create type parameter environment first if this is a generic function
		var typeParamEnv *Environment = originalEnv
		if len(funcLit.TypeParameters) > 0 {
			// Create a new environment that includes type parameters
			typeParamEnv = NewEnclosedEnvironment(originalEnv)

			// Extract existing type parameters from the hoisted signature
			typeParamsFromSignature := c.extractTypeParametersFromSignature(funcSignature)

			// Define each type parameter in the environment using the original instances
			for i, typeParamNode := range funcLit.TypeParameters {
				var typeParam *types.TypeParameter

				// Find the matching type parameter from the signature
				if i < len(typeParamsFromSignature) {
					typeParam = typeParamsFromSignature[i]
				} else {
					// Fallback: create new one (shouldn't happen if hoisting worked correctly)
					typeParam = &types.TypeParameter{
						Name:       typeParamNode.Name.Value,
						Constraint: types.Any,
						Default:    nil, // Fallback case - no default
						Index:      i,
					}
					debugPrintf("// [Checker Pass 3] WARNING: Had to create new type parameter '%s'\n", typeParam.Name)
				}

				// Define it in the environment
				// Check if it already exists to avoid false duplicates during multiple processing
				if existing, exists := typeParamEnv.ResolveTypeParameter(typeParam.Name); exists {
					// Already defined, reuse the existing one
					typeParamNode.SetComputedType(&types.TypeParameterType{Parameter: existing})
				} else if !typeParamEnv.DefineTypeParameter(typeParam.Name, typeParam) {
					c.addError(typeParamNode.Name, fmt.Sprintf("duplicate type parameter name: %s", typeParam.Name))
				} else {
					// Successfully defined, set computed type on the AST node
					typeParamNode.SetComputedType(&types.TypeParameterType{Parameter: typeParam})
				}

				debugPrintf("// [Checker Pass 3] Defined type parameter '%s' for body checking (reused from hoisting)\n", typeParam.Name)
			}
		}

		funcEnv := NewFunctionEnvironment(typeParamEnv)
		c.env = funcEnv
		// Define parameters using the initial signature
		for i, paramNode := range funcLit.Parameters {
			if i < len(funcSignature.ParameterTypes) {
				paramType := funcSignature.ParameterTypes[i]
				// Skip 'this' parameters as they don't have names and don't go into the scope
				if !paramNode.IsThis {
					if !funcEnv.Define(paramNode.Name.Value, paramType, false) {
						c.addError(paramNode.Name, fmt.Sprintf("duplicate parameter name: %s", paramNode.Name.Value))
					}
				}
				paramNode.ComputedType = paramType // Set type on parameter node
			} else {
				debugPrintf("// [Checker Pass 3] ERROR: Param count mismatch for func '%s'\n", funcNameForLog)
			}
		}

		// --- NEW: Define rest parameter if present ---
		if funcLit.RestParameter != nil && funcSignature.RestParameterType != nil {
			// Check if it's a simple identifier or destructuring pattern
			if funcLit.RestParameter.Name != nil {
				// Simple rest parameter like ...args
				if !funcEnv.Define(funcLit.RestParameter.Name.Value, funcSignature.RestParameterType, false) {
					c.addError(funcLit.RestParameter.Name, fmt.Sprintf("duplicate parameter name: %s", funcLit.RestParameter.Name.Value))
				}
				debugPrintf("// [Checker Pass 3] Defined rest parameter '%s' with type: %s\n", funcLit.RestParameter.Name.Value, funcSignature.RestParameterType.String())
			} else if funcLit.RestParameter.Pattern != nil {
				// Destructuring rest parameter like ...[x, y] or ...{a, b}
				// Extract element type from the rest array type (which is an array of T)
				var elementType types.Type = types.Any
				if arrayType, ok := funcSignature.RestParameterType.(*types.ArrayType); ok {
					elementType = arrayType.ElementType
				}

				// Define the pattern variables with the element type
				switch pattern := funcLit.RestParameter.Pattern.(type) {
				case *parser.ArrayParameterPattern:
					// Define each element in the array pattern
					for _, elem := range pattern.Elements {
						if elem != nil && elem.Target != nil {
							if ident, ok := elem.Target.(*parser.Identifier); ok {
								if !funcEnv.Define(ident.Value, elementType, false) {
									c.addError(ident, fmt.Sprintf("duplicate parameter name: %s", ident.Value))
								}
								debugPrintf("// [Checker Pass 3] Defined rest destructured param '%s' with type: %s\n", ident.Value, elementType.String())
							}
						}
					}
				case *parser.ObjectParameterPattern:
					// Define each property in the object pattern
					for _, prop := range pattern.Properties {
						if prop != nil && prop.Key != nil {
							if ident, ok := prop.Key.(*parser.Identifier); ok {
								if !funcEnv.Define(ident.Value, elementType, false) {
									c.addError(ident, fmt.Sprintf("duplicate parameter name: %s", ident.Value))
								}
								debugPrintf("// [Checker Pass 3] Defined rest destructured prop '%s' with type: %s\n", ident.Value, elementType.String())
							}
						}
					}
				}
				debugPrintf("// [Checker Pass 3] Rest parameter with destructuring pattern (type: %s)\n", funcSignature.RestParameterType.String())
			}
			// Set computed type on the RestParameter node itself
			funcLit.RestParameter.ComputedType = funcSignature.RestParameterType
		}
		// --- END NEW ---

		// Define function itself within its scope for recursion (using initial signature)
		if funcLit.Name != nil {
			funcEnv.Define(funcLit.Name.Value, funcObjectType, false) // Ignore error if already defined (e.g. hoisted)
		}

		// Visit Body
		c.visit(funcLit.Body) // Use funcEnv implicitly

		// Determine Final ACTUAL Return Type
		var actualReturnType types.Type
		if funcSignature.ReturnType != nil { // Annotation existed
			actualReturnType = funcSignature.ReturnType
		} else { // No annotation, infer
			if len(c.currentInferredReturnTypes) == 0 {
				actualReturnType = types.Void // Functions with no return statements have void return type
			} else {
				actualReturnType = types.NewUnionType(c.currentInferredReturnTypes...)
			}
			debugPrintf("// [Checker Pass 3] Inferred return type for '%s': %s\n", funcNameForLog, actualReturnType.String())
		}

		// Handle async generator functions (both flags) first
		if funcLit.IsAsync && funcLit.IsGenerator {
			debugPrintf("// [Checker Pass 3] Async generator function detected, wrapping return type in AsyncGenerator\n")
			asyncGeneratorType := c.createAsyncGeneratorType(actualReturnType, c.currentInferredYieldTypes)
			actualReturnType = asyncGeneratorType
			debugPrintf("// [Checker Pass 3] Wrapped return type: %s\n", actualReturnType.String())
		} else if funcLit.IsGenerator {
			// For generator functions, wrap the return type in Generator<T, TReturn, TNext>
			debugPrintf("// [Checker Pass 3] Generator function detected, wrapping return type in Generator\n")
			generatorType := c.createGeneratorType(actualReturnType, c.currentInferredYieldTypes)
			actualReturnType = generatorType
			debugPrintf("// [Checker Pass 3] Wrapped return type: %s\n", actualReturnType.String())
		} else if funcLit.IsAsync {
			// For async functions, wrap the return type in Promise<T>
			debugPrintf("// [Checker Pass 3] Async function detected, wrapping return type in Promise\n")
			innerType := actualReturnType
			if innerType == nil {
				innerType = types.Void
			}
			promiseType := c.createPromiseType(innerType)
			actualReturnType = promiseType
			debugPrintf("// [Checker Pass 3] Wrapped return type: %s\n", actualReturnType.String())
		}

		// Create the FINAL ObjectType with updated signature
		finalSignature := &types.Signature{
			ParameterTypes:    funcSignature.ParameterTypes,
			ReturnType:        actualReturnType,
			OptionalParams:    funcSignature.OptionalParams,
			IsVariadic:        funcSignature.IsVariadic,        // Add variadic info
			RestParameterType: funcSignature.RestParameterType, // Add rest parameter type
		}
		finalFuncType := types.NewFunctionType(finalSignature)

		// *** Update Environment & Node ***
		// Update the function's type in the *outer* (global) environment
		targetName := ""
		if funcLit.Name != nil { // Hoisted function
			targetName = funcLit.Name.Value
		} else { // Find the variable it was assigned to
			// This requires linking back from funcLit node to the Let/Const stmt, which is tricky.
			// Alternative: Update based on the node pointer? Need a map[Node]Name...
			// For now, let's only update hoisted functions explicitly by name.
			// Assigned functions will rely on the final check pass.
			// TODO: Fix this update logic for assigned functions.
		}

		if targetName != "" {
			if !globalEnv.Update(targetName, finalFuncType) {
				debugPrintf("// [Checker Pass 3] WARNING: Failed global env update for '%s'\n", targetName)
			} else {
				debugPrintf("// [Checker Pass 3] Updated global env for '%s' to final type: %s\n", targetName, finalFuncType.String())
			}

			// --- NEW: Check for overload completion ---
			if len(globalEnv.GetPendingOverloads(targetName)) > 0 {
				debugPrintf("// [Checker Pass 3] Found pending overloads for '%s', completing overloaded function\n", targetName)
				// Use unified ObjectType directly
				c.completeOverloadedFunction(targetName, finalFuncType)
			}
			// --- END NEW ---
		} else {
			debugPrintf("// [Checker Pass 3] Skipping global env update for anonymous/assigned func %p\n", funcLit)
		}

		// ALWAYS set the final computed type on the FunctionLiteral node itself
		debugPrintf("// [Checker Pass 3] SETTING final computed type on node %p for '%s': %s\n", funcLit, funcNameForLog, finalFuncType.String())
		funcLit.SetComputedType(finalFuncType)

		// Restore outer environment and context
		c.env = originalEnv
		c.currentExpectedReturnType = outerExpectedReturnType
		c.currentInferredReturnTypes = outerInferredReturnTypes
		c.currentInferredYieldTypes = outerInferredYieldTypes
		c.currentThisType = outerThisType
		c.inAsyncFunction = outerInAsyncFunction
		c.inGeneratorFunction = outerInGeneratorFunction
		c.functionNestingDepth--
	}
	debugPrintf("// --- Checker - Pass 3: Complete ---\n")

	// --- Pass 4: Resolve forward references ---
	debugPrintf("\n// --- Checker - Pass 4: Resolve Forward References ---\n")
	c.resolveForwardReferences()

	// --- Pass 5: Final Check of Remaining Statements & Initializers ---
	debugPrintf("\n// --- Checker - Pass 5: Final Checks & Remaining Statements ---\n")
	c.env = globalEnv // Ensure global scope
	for _, stmt := range program.Statements {
		// Skip nodes processed in initial passes OR function literals visited in Pass 3
		if nodesProcessedPass1[stmt] || nodesProcessedPass2[stmt] {
			// Special check: If it's a Let/Const whose VALUE was a FunctionLiteral,
			// we marked the STATEMENT in Pass 2, but we still need to check its initializer assignability here.
			needsInitializerCheck := false
			switch specificNode := stmt.(type) {
			case *parser.LetStatement:
				if _, ok := specificNode.Value.(*parser.FunctionLiteral); !ok && specificNode.Value != nil {
					needsInitializerCheck = true
				}
			case *parser.ConstStatement:
				if _, ok := specificNode.Value.(*parser.FunctionLiteral); !ok && specificNode.Value != nil {
					needsInitializerCheck = true
				}
			case *parser.VarStatement:
				if _, ok := specificNode.Value.(*parser.FunctionLiteral); !ok && specificNode.Value != nil {
					needsInitializerCheck = true
				}
			}

			if !needsInitializerCheck {
				debugPrintf("// [Checker Pass 5] Skipping already processed/visited node: %T\n", stmt)
				continue
			} else {
				debugPrintf("// [Checker Pass 5] Re-visiting Let/Const/Var for initializer check: %T\n", stmt)
			}
		}

		debugPrintf("// [Checker Pass 5] Visiting node: %T\n", stmt)

		// Re-visit Let/Const/Var to check initializers OR visit other statements like ExpressionStatement
		switch node := stmt.(type) {
		case *parser.LetStatement, *parser.ConstStatement, *parser.VarStatement:
			// This block now ONLY handles checking non-function initializers
			var varName *parser.Identifier
			var typeAnnotation parser.Expression // Added to check if annotation existed
			var initializer parser.Expression
			// isConst := false // Removed, not needed here

			switch specificNode := node.(type) {
			case *parser.LetStatement:
				varName, typeAnnotation, initializer = specificNode.Name, specificNode.TypeAnnotation, specificNode.Value
			case *parser.ConstStatement:
				varName, typeAnnotation, initializer = specificNode.Name, specificNode.TypeAnnotation, specificNode.Value //; isConst = true
			case *parser.VarStatement:
				varName, typeAnnotation, initializer = specificNode.Name, specificNode.TypeAnnotation, specificNode.Value
			}

			if initializer != nil {
				// Initializer exists and wasn't a function literal handled before
				debugPrintf("// [Checker Pass 5] Checking initializer for variable '%s'\n", varName.Value)

				// Get the variable's type defined in Pass 2 to check if we have a type annotation
				variableType, _, found := globalEnv.Resolve(varName.Value)
				if !found { // Should not happen
					debugPrintf("// [Checker Pass 5] ERROR: Variable '%s' not found in env during final check?\n", varName.Value)
					continue
				}

				// Use contextual typing if we have a type annotation (not Any)
				if typeAnnotation != nil && variableType != types.Any {
					debugPrintf("// [Checker Pass 5] Using contextual typing for '%s' with expected type: %s\n", varName.Value, variableType.String())
					c.visitWithContext(initializer, &ContextualType{
						ExpectedType: variableType,
						IsContextual: true,
					})
				} else {
					c.visit(initializer) // Regular visit if no type annotation
				}

				computedInitializerType := initializer.GetComputedType()
				if computedInitializerType == nil {
					computedInitializerType = types.Any
				}

				// Perform assignability check using the type from env (e.g., Any or annotation)
				// Use expansion to handle mapped types
				assignable := c.isAssignableWithExpansion(computedInitializerType, variableType)

				// Handle special case for assigning [] (unknown[]) to T[]
				isEmptyArrayAssignment := false
				if _, isTargetArray := variableType.(*types.ArrayType); isTargetArray {
					if sourceArray, isSourceArray := computedInitializerType.(*types.ArrayType); isSourceArray {
						if sourceArray.ElementType == types.Unknown {
							isEmptyArrayAssignment = true
						}
					}
				}
				// Allow assigning empty array even if assignable check fails due to unknown element type
				if isEmptyArrayAssignment {
					assignable = true
				}

				// Additional validation: Check index signature constraints
				if assignable { // Only check index signatures if basic assignability passes
					indexSigErrors := c.validateIndexSignatures(computedInitializerType, variableType)
					for _, sigError := range indexSigErrors {
						c.addError(initializer, fmt.Sprintf("Type '%s' is not assignable to type '%s' as required by index signature [%s: %s]",
							sigError.PropertyType.String(), sigError.ExpectedType.String(),
							sigError.KeyType.String(), sigError.ExpectedType.String()))
					}
					// If there are index signature errors, treat assignment as invalid
					if len(indexSigErrors) > 0 {
						assignable = false
					}
				}

				if !assignable {
					// Check if this is an enum assignment to use appropriate error format
					if c.isEnumType(variableType) {
						// For enum assignments, use widened source type and no variable name
						sourceTypeStr, targetTypeStr := c.getEnumAssignmentErrorTypes(computedInitializerType, variableType)
						c.addError(initializer, fmt.Sprintf("cannot assign type '%s' to variable of type '%s'", sourceTypeStr, targetTypeStr))
					} else {
						// For regular variable assignments, use literal types and include variable name
						sourceTypeStr, targetTypeStr := c.getAssignmentErrorTypes(computedInitializerType, variableType)
						variableName := varName.Value
						c.addError(initializer, fmt.Sprintf("cannot assign type '%s' to variable '%s' of type '%s'", sourceTypeStr, variableName, targetTypeStr))
					}
				}

				// --- FIX: Refine variable type in environment if no annotation ---
				if typeAnnotation == nil && found { // Check if ANNOTATION was nil
					// Widen literal types before updating environment, unless it's empty array
					var finalInferredType types.Type
					if isEmptyArrayAssignment {
						finalInferredType = computedInitializerType // Keep unknown[] type
					} else {
						finalInferredType = types.DeeplyWidenType(computedInitializerType) // Use the deep widen helper
					}

					// Update the environment only if the refined type is different from the current one
					if variableType != finalInferredType {
						debugPrintf("// [Checker Pass 5] Refining type for '%s' (no annotation). Old: %s, New: %s\n", varName.Value, variableType.String(), finalInferredType.String())
						if !globalEnv.Update(varName.Value, finalInferredType) {
							debugPrintf("// [Checker Pass 5] WARNING: Failed env update refinement for '%s'\n", varName.Value)
						}
						// Also update the type on the name node itself for consistency
						varName.SetComputedType(finalInferredType)
					} else {
						debugPrintf("// [Checker Pass 5] Type for '%s' already refined to %s. No update needed.\n", varName.Value, variableType.String())
					}
				}
				// --- END FIX ---
			}

		case *parser.ExpressionStatement:
			c.visit(node.Expression)

			// Special handling for FunctionSignature expressions
			if sigExpr, ok := node.Expression.(*parser.FunctionSignature); ok {
				c.processFunctionSignature(sigExpr)
			}

		// TODO: Handle other top-level statement types if necessary
		default:
			debugPrintf("// [Checker Pass 5] Visiting unhandled statement type %T\n", node)
			c.visit(node) // Fallback visit? Might be unnecessary
		}
	}
	debugPrintf("// --- Checker - Pass 5: Complete ---\n")

	return c.errors
}

// resolveForwardReferences resolves typeof forward references after variables have been defined
func (c *Checker) resolveForwardReferences() {
	debugPrintf("// [Checker resolveForwardReferences] Starting forward reference resolution\n")

	// Walk through all type aliases and resolve any TypeofType forward references
	// We need to iterate through the environment to find type aliases
	c.resolveTypeofForwardReferences(c.env)

	// After resolving type aliases, we need to update any variable types that reference resolved aliases
	c.updateVariableTypesAfterAliasResolution(c.env)
}

// resolveTypeofForwardReferences recursively resolves typeof forward references in the environment
func (c *Checker) resolveTypeofForwardReferences(env *Environment) {
	// We need to resolve TypeofType instances in type aliases
	// This requires walking through the environment's type aliases and replacing TypeofType with actual types

	// Since we can't easily iterate through environment internals,
	// let's use a visitor pattern to find and resolve TypeofType instances
	c.resolveTypeofInEnvironment(env)
}

// resolveTypeofInEnvironment resolves typeof types in the given environment
func (c *Checker) resolveTypeofInEnvironment(env *Environment) {
	debugPrintf("// [Checker resolveTypeofInEnvironment] Starting typeof resolution\n")

	// Get all type aliases from the environment
	aliases := env.GetAllTypeAliases()
	debugPrintf("// [Checker resolveTypeofInEnvironment] Found %d type aliases to check\n", len(aliases))

	// For each type alias, check if it contains TypeofType and resolve it
	for name, aliasType := range aliases {
		debugPrintf("// [Checker resolveTypeofInEnvironment] Checking alias '%s': %T\n", name, aliasType)

		// Recursively resolve any TypeofType instances in the alias
		resolvedType := c.resolveTypeofInType(aliasType)
		if resolvedType != aliasType {
			debugPrintf("// [Checker resolveTypeofInEnvironment] Updated alias '%s' from %s to %s\n",
				name, aliasType.String(), resolvedType.String())
			env.DefineTypeAlias(name, resolvedType)
		}
	}
}

// resolveTypeofInType recursively resolves TypeofType instances in a type
func (c *Checker) resolveTypeofInType(t types.Type) types.Type {
	if t == nil {
		return nil
	}

	switch typ := t.(type) {
	case *types.TypeofType:
		// Try to resolve the typeof type
		return c.resolveTypeofTypeIfNeeded(typ)

	case *types.ConditionalType:
		// Recursively resolve in conditional type components
		resolvedCheckType := c.resolveTypeofInType(typ.CheckType)
		resolvedExtendsType := c.resolveTypeofInType(typ.ExtendsType)
		resolvedTrueType := c.resolveTypeofInType(typ.TrueType)
		resolvedFalseType := c.resolveTypeofInType(typ.FalseType)

		// If any component changed, re-evaluate the conditional type
		if resolvedCheckType != typ.CheckType || resolvedExtendsType != typ.ExtendsType ||
			resolvedTrueType != typ.TrueType || resolvedFalseType != typ.FalseType {
			debugPrintf("// [resolveTypeofInType] Re-evaluating conditional type after typeof resolution\n")
			// Re-evaluate the conditional type with resolved typeof types
			return c.computeConditionalType(resolvedCheckType, resolvedExtendsType, resolvedTrueType, resolvedFalseType)
		}

	case *types.ArrayType:
		resolvedElementType := c.resolveTypeofInType(typ.ElementType)
		if resolvedElementType != typ.ElementType {
			return &types.ArrayType{ElementType: resolvedElementType}
		}

	case *types.UnionType:
		changed := false
		resolvedTypes := make([]types.Type, len(typ.Types))
		for i, memberType := range typ.Types {
			resolved := c.resolveTypeofInType(memberType)
			resolvedTypes[i] = resolved
			if resolved != memberType {
				changed = true
			}
		}
		if changed {
			return types.NewUnionType(resolvedTypes...)
		}

		// Add more cases as needed for other composite types
	}

	// Return the original type if no TypeofType was found
	return t
}

// containsUnresolvedTypeofType checks if a type contains unresolved TypeofType instances
func (c *Checker) containsUnresolvedTypeofType(t types.Type) bool {
	if t == nil {
		return false
	}

	switch typ := t.(type) {
	case *types.TypeofType:
		// Check if this typeof can be resolved right now
		resolvedType := c.resolveTypeofTypeIfNeeded(typ)
		// If it's still a TypeofType, it's unresolved
		_, stillUnresolved := resolvedType.(*types.TypeofType)
		return stillUnresolved

	case *types.ConditionalType:
		// Check recursively in conditional type components
		return c.containsUnresolvedTypeofType(typ.CheckType) ||
			c.containsUnresolvedTypeofType(typ.ExtendsType) ||
			c.containsUnresolvedTypeofType(typ.TrueType) ||
			c.containsUnresolvedTypeofType(typ.FalseType)

	case *types.ArrayType:
		return c.containsUnresolvedTypeofType(typ.ElementType)

	case *types.UnionType:
		for _, memberType := range typ.Types {
			if c.containsUnresolvedTypeofType(memberType) {
				return true
			}
		}

		// Add more cases as needed for other composite types
	}

	return false
}

// updateVariableTypesAfterAliasResolution updates variable types that reference resolved type aliases
func (c *Checker) updateVariableTypesAfterAliasResolution(env *Environment) {
	debugPrintf("// [Checker updateVariableTypesAfterAliasResolution] Starting variable type updates\n")

	// Get all variables from the environment
	variables := env.GetAllVariables()
	debugPrintf("// [Checker updateVariableTypesAfterAliasResolution] Found %d variables to check\n", len(variables))

	// For each variable, check if its type needs to be updated
	for name, symbolInfo := range variables {
		debugPrintf("// [Checker updateVariableTypesAfterAliasResolution] Checking variable '%s': %T\n", name, symbolInfo.Type)

		// Check if the variable's type contains unresolved conditional types or type aliases
		updatedType := c.resolveTypeofInType(symbolInfo.Type)
		if updatedType != symbolInfo.Type {
			debugPrintf("// [Checker updateVariableTypesAfterAliasResolution] Updated variable '%s' from %s to %s\n",
				name, symbolInfo.Type.String(), updatedType.String())
			// Update the variable's type in the environment
			env.Update(name, updatedType)
		}
	}
}

// visit is the main dispatch method for AST traversal (Visitor pattern lite).
func (c *Checker) visit(node parser.Node) {
	if node == nil {
		return
	}
	debugPrintf("// [DEBUG TEST] This is a debug test message\n")
	debugPrintf("// [Checker Visit Enter] Node: %T, Env: %p\n", node, c.env)
	//fmt.Printf("DEBUG: Visiting node type: %T\n", node)

	switch node := node.(type) {
	case *parser.Program:
		for _, stmt := range node.Statements {
			c.visit(stmt)
		}

	case *parser.TypeAliasStatement:
		c.checkTypeAliasStatement(node)

	case *parser.InterfaceDeclaration:
		c.checkInterfaceDeclaration(node)

	case *parser.FunctionSignature:
		// Process function overload signatures
		c.processFunctionSignature(node)

	case *parser.ExpressionStatement:
		c.visit(node.Expression)

		// Special handling for FunctionSignature expressions
		if sigExpr, ok := node.Expression.(*parser.FunctionSignature); ok {
			c.processFunctionSignature(sigExpr)
		}

	case *parser.LetStatement:
		// Process all variable declarations in the statement
		if node.Declarations == nil || len(node.Declarations) == 0 {
			// Fallback to legacy fields if Declarations is not set
			if node.Name != nil {
				panic(fmt.Sprintf("PANIC: LetStatement has Name but no Declarations: %s", node.Name.Value))
			}
			return
		}
		for i, declarator := range node.Declarations {
			// Set current declarator in legacy fields for backward compatibility
			node.Name = declarator.Name
			node.TypeAnnotation = declarator.TypeAnnotation
			node.Value = declarator.Value
			node.ComputedType = declarator.ComputedType

			var nameValueStr string
			if declarator.Name != nil {
				nameValueStr = declarator.Name.Value
			} else {
				nameValueStr = "<nil_name>"
			}
			debugPrintf("// [Checker LetStmt Entry %d] Name: %s\n", i, nameValueStr)

			// 1. Handle Type Annotation (if present)
			var declaredType types.Type
			if declarator.TypeAnnotation != nil {
				declaredType = c.resolveTypeAnnotation(declarator.TypeAnnotation)
			} else {
				declaredType = nil // Indicates type inference is needed
			}

			// --- FIX V2 for recursive functions assigned to variables ---
			// Determine a preliminary type for the initializer if it's a function literal.
			var preliminaryInitializerType types.Type = nil
			if funcLit, ok := declarator.Value.(*parser.FunctionLiteral); ok {
				// Extract param types (default Any)
				prelimParamTypes := []types.Type{}
				for _, p := range funcLit.Parameters {
					if p.TypeAnnotation != nil {
						pt := c.resolveTypeAnnotation(p.TypeAnnotation)
						if pt != nil {
							prelimParamTypes = append(prelimParamTypes, pt)
						} else {
							prelimParamTypes = append(prelimParamTypes, types.Any) // Error resolving, use Any
						}
					} else {
						prelimParamTypes = append(prelimParamTypes, types.Any) // No annotation, use Any
					}
				}
				// Extract return type annotation (can be nil)
				prelimReturnType := c.resolveTypeAnnotation(funcLit.ReturnTypeAnnotation)
				// Create unified ObjectType for preliminary function type
				prelimSig := &types.Signature{
					ParameterTypes: prelimParamTypes,
					ReturnType:     prelimReturnType,
				}
				preliminaryInitializerType = types.NewFunctionType(prelimSig)
			}

			// Temporarily define the variable name in the current scope before visiting the value.
			tempType := preliminaryInitializerType
			if tempType == nil {
				tempType = declaredType // Use annotation if no prelim func type
			}
			if tempType == nil {
				tempType = types.Any // Fallback to Any
			}
			if !c.env.Define(declarator.Name.Value, tempType, false) {
				// If Define fails here, it's a true redeclaration error.
				c.addError(declarator.Name, fmt.Sprintf("variable '%s' already declared in this scope", declarator.Name.Value))
			}
			debugPrintf("// [Checker LetStmt] Temp Define '%s' as: %s\n", declarator.Name.Value, tempType.String())

			// 2. Handle Initializer (if present)
			var computedInitializerType types.Type
			if declarator.Value != nil {
				// Use contextual typing if we have a declared type
				if declaredType != nil {
					c.visitWithContext(declarator.Value, &ContextualType{
						ExpectedType: declaredType,
						IsContextual: true,
					})
				} else {
					c.visit(declarator.Value) // Regular visit if no type annotation
				}

				computedInitializerType = declarator.Value.GetComputedType()
				debugPrintf("// [Checker LetStmt] '%s': computedInitializerType from declarator.Value (%T): %T (%v)\n", nameValueStr, declarator.Value, computedInitializerType, computedInitializerType)
			} else {
				computedInitializerType = nil // No initializer
			}

			// 3. Determine the final type and check assignment errors
			var finalVariableType types.Type

			if declaredType != nil {
				// --- If annotation exists, variable type IS the annotation ---
				finalVariableType = declaredType

				if computedInitializerType != nil {
					// Check if initializer is assignable to the declared type for error reporting
					assignable := c.isAssignableWithExpansion(computedInitializerType, declaredType)

					// --- SPECIAL CASE: Allow assignment of [] (unknown[]) to T[] ---
					isEmptyArrayAssignment := false
					if _, isTargetArray := declaredType.(*types.ArrayType); isTargetArray {
						if sourceArrayType, isSourceArray := computedInitializerType.(*types.ArrayType); isSourceArray {
							if sourceArrayType.ElementType == types.Unknown {
								isEmptyArrayAssignment = true
							}
						}
					}

					if !assignable && !isEmptyArrayAssignment {
						// Check if this is an enum assignment to use appropriate error format
						if c.isEnumType(finalVariableType) {
							// For enum assignments, use widened source type and no variable name
							sourceTypeStr, targetTypeStr := c.getEnumAssignmentErrorTypes(computedInitializerType, finalVariableType)
							c.addError(declarator.Value, fmt.Sprintf("cannot assign type '%s' to variable of type '%s'", sourceTypeStr, targetTypeStr))
						} else {
							// For regular variable assignments, use literal types and include variable name
							sourceTypeStr, targetTypeStr := c.getAssignmentErrorTypes(computedInitializerType, finalVariableType)
							variableName := nameValueStr
							c.addError(declarator.Value, fmt.Sprintf("cannot assign type '%s' to variable '%s' of type '%s'", sourceTypeStr, variableName, targetTypeStr))
						}
					}
				}
			} else {
				// --- No annotation: Infer type ---
				if computedInitializerType != nil {
					if _, isLiteral := computedInitializerType.(*types.LiteralType); isLiteral {
						finalVariableType = types.GetWidenedType(computedInitializerType)
						debugPrintf("// [Checker LetStmt] '%s': Inferred final type (widened literal): %s (Go Type: %T)\n", nameValueStr, finalVariableType.String(), finalVariableType)
					} else {
						finalVariableType = computedInitializerType
						debugPrintf("// [Checker LetStmt] '%s': Assigned finalVariableType (direct non-literal): %s (Go Type: %T)\n", nameValueStr, finalVariableType.String(), finalVariableType)
					}
				} else {
					finalVariableType = types.Undefined
					debugPrintf("// [Checker LetStmt] '%s': Inferred final type (no initializer): %s (Go Type: %T)\n", nameValueStr, finalVariableType.String(), finalVariableType)
				}
			}

			// 4. UPDATE variable type in the current environment with the final type
			currentType, _, _ := c.env.Resolve(declarator.Name.Value)
			debugPrintf("// [Checker LetStmt] Updating '%s'. Current type: %s, Final type: %s\n", declarator.Name.Value, currentType.String(), finalVariableType.String())
			if !c.env.Update(declarator.Name.Value, finalVariableType) {
				debugPrintf("// [Checker LetStmt] WARNING: Update failed unexpectedly for '%s'\n", declarator.Name.Value)
			}
			updatedType, _, _ := c.env.Resolve(declarator.Name.Value)
			debugPrintf("// [Checker LetStmt] Updated '%s'. Type after update: %s\n", declarator.Name.Value, updatedType.String())

			// Set computed type on the Name Identifier node itself and the declarator
			declarator.Name.SetComputedType(finalVariableType)
			declarator.ComputedType = finalVariableType
		}

	case *parser.ConstStatement:
		// Process all constant declarations in the statement
		for i, declarator := range node.Declarations {
			// Set current declarator in legacy fields for backward compatibility
			node.Name = declarator.Name
			node.TypeAnnotation = declarator.TypeAnnotation
			node.Value = declarator.Value
			node.ComputedType = declarator.ComputedType

			var nameValueStr string
			if declarator.Name != nil {
				nameValueStr = declarator.Name.Value
			} else {
				nameValueStr = "<nil_name>"
			}
			debugPrintf("// [Checker ConstStmt Entry %d] Name: %s\n", i, nameValueStr)

			// 1. Handle Type Annotation (if present)
			var declaredType types.Type
			if declarator.TypeAnnotation != nil {
				declaredType = c.resolveTypeAnnotation(declarator.TypeAnnotation)
			} else {
				declaredType = nil // Indicates type inference is needed
			}

			// 2. Handle Initializer (Must be present for const)
			var computedInitializerType types.Type
			if declarator.Value != nil {
				// Use contextual typing if we have a declared type
				if declaredType != nil {
					c.visitWithContext(declarator.Value, &ContextualType{
						ExpectedType: declaredType,
						IsContextual: true,
					})
				} else {
					c.visit(declarator.Value) // Regular visit if no type annotation
				}
				computedInitializerType = declarator.Value.GetComputedType()
			} else {
				// Constants MUST be initialized
				c.addError(declarator.Name, fmt.Sprintf("const declaration '%s' must be initialized", declarator.Name.Value))
				computedInitializerType = types.Any // Assign Any to prevent further cascading errors downstream
			}

			// 3. Determine the final type and check assignment errors
			var finalType types.Type

			if declaredType != nil {
				// --- If annotation exists, constant type IS the annotation ---
				finalType = declaredType

				// Check if initializer is assignable to the declared type for error reporting
				assignable := c.isAssignableWithExpansion(computedInitializerType, declaredType)

				// --- SPECIAL CASE: Allow assignment of [] (unknown[]) to T[] ---
				isEmptyArrayAssignment := false
				if _, isTargetArray := declaredType.(*types.ArrayType); isTargetArray {
					if sourceArrayType, isSourceArray := computedInitializerType.(*types.ArrayType); isSourceArray {
						if sourceArrayType.ElementType == types.Unknown {
							isEmptyArrayAssignment = true
						}
					}
				}

				if !assignable && !isEmptyArrayAssignment {
					c.addError(declarator.Value, fmt.Sprintf("cannot assign type '%s' to constant '%s' of type '%s'", computedInitializerType.String(), declarator.Name.Value, finalType.String()))
				}
			} else {
				// --- No annotation: Infer type ---
				// computedInitializerType should not be nil here due to const requirement check above

				// Only widen literal types
				if _, isLiteral := computedInitializerType.(*types.LiteralType); isLiteral {
					finalType = types.GetWidenedType(computedInitializerType)
				} else {
					// Use the computed type directly for non-literals (functions, arrays, etc.)
					finalType = computedInitializerType
				}
			}

			// 4. Define variable in the current environment
			if !c.env.Define(declarator.Name.Value, finalType, true) {
				c.addError(declarator.Name, fmt.Sprintf("constant '%s' already declared in this scope", declarator.Name.Value))
			}
			// Set computed type on the Name Identifier node itself and the declarator
			declarator.Name.SetComputedType(finalType)
			declarator.ComputedType = finalType
		}

	case *parser.VarStatement:
		// Process all variable declarations in the statement
		for i, declarator := range node.Declarations {
			// Set current declarator in legacy fields for backward compatibility
			node.Name = declarator.Name
			node.TypeAnnotation = declarator.TypeAnnotation
			node.Value = declarator.Value
			node.ComputedType = declarator.ComputedType

			var nameValueStr string
			if declarator.Name != nil {
				nameValueStr = declarator.Name.Value
			} else {
				nameValueStr = "<nil_name>"
			}
			debugPrintf("// [Checker VarStmt Entry %d] Name: %s\n", i, nameValueStr)

			// 1. Handle Type Annotation (if present)
			var declaredType types.Type
			if declarator.TypeAnnotation != nil {
				declaredType = c.resolveTypeAnnotation(declarator.TypeAnnotation)
			} else {
				declaredType = nil // Indicates type inference is needed
			}

			// --- FIX V2 for recursive functions assigned to variables ---
			// Determine a preliminary type for the initializer if it's a function literal.
			var preliminaryInitializerType types.Type = nil
			if funcLit, ok := declarator.Value.(*parser.FunctionLiteral); ok {
				// Extract param types (default Any)
				prelimParamTypes := []types.Type{}
				for _, p := range funcLit.Parameters {
					if p.TypeAnnotation != nil {
						pt := c.resolveTypeAnnotation(p.TypeAnnotation)
						if pt != nil {
							prelimParamTypes = append(prelimParamTypes, pt)
						} else {
							prelimParamTypes = append(prelimParamTypes, types.Any) // Error resolving, use Any
						}
					} else {
						prelimParamTypes = append(prelimParamTypes, types.Any) // No annotation, use Any
					}
				}
				// Extract return type annotation (can be nil)
				prelimReturnType := c.resolveTypeAnnotation(funcLit.ReturnTypeAnnotation)
				// Create unified ObjectType for preliminary function type
				prelimSig := &types.Signature{
					ParameterTypes: prelimParamTypes,
					ReturnType:     prelimReturnType,
				}
				preliminaryInitializerType = types.NewFunctionType(prelimSig)
			}

			// Temporarily define the variable name in the current scope before visiting the value.
			tempType := preliminaryInitializerType
			if tempType == nil {
				tempType = declaredType // Use annotation if no prelim func type
			}
			if tempType == nil {
				tempType = types.Any // Fallback to Any
			}
			// var declarations should be defined in function scope, not block scope (var hoisting)
			funcScope := c.env.GetFunctionScope()
			if !funcScope.Define(declarator.Name.Value, tempType, false) {
				// For var, redeclaration in the same scope is allowed (it just reassigns)
				// So if Define fails, check if it already exists - if so, that's OK for var
				_, _, exists := funcScope.Resolve(declarator.Name.Value)
				if !exists {
					// Variable doesn't exist but Define failed - this shouldn't happen
					c.addError(declarator.Name, fmt.Sprintf("variable '%s' already declared in this scope", declarator.Name.Value))
				}
				// If exists, silently allow redeclaration (JavaScript var semantics)
			} else {
				debugPrintf("// [Checker VarStmt] Temp Define '%s' as: %s in function scope\n", declarator.Name.Value, tempType.String())
			}

			// 2. Handle Initializer (if present)
			var computedInitializerType types.Type
			if declarator.Value != nil {
				// Use contextual typing if we have a declared type
				if declaredType != nil {
					c.visitWithContext(declarator.Value, &ContextualType{
						ExpectedType: declaredType,
						IsContextual: true,
					})
				} else {
					c.visit(declarator.Value) // Regular visit if no type annotation
				}

				computedInitializerType = declarator.Value.GetComputedType()
				debugPrintf("// [Checker VarStmt] '%s': computedInitializerType from declarator.Value (%T): %T (%v)\n", nameValueStr, declarator.Value, computedInitializerType, computedInitializerType)
			} else {
				computedInitializerType = nil // No initializer
			}

			// 3. Determine the final type and check assignment errors
			var finalVariableType types.Type

			if declaredType != nil {
				// --- If annotation exists, variable type IS the annotation ---
				finalVariableType = declaredType

				if computedInitializerType != nil {
					// Check if initializer is assignable to the declared type for error reporting
					assignable := c.isAssignableWithExpansion(computedInitializerType, declaredType)

					// --- SPECIAL CASE: Allow assignment of [] (unknown[]) to T[] ---
					isEmptyArrayAssignment := false
					if _, isTargetArray := declaredType.(*types.ArrayType); isTargetArray {
						if sourceArrayType, isSourceArray := computedInitializerType.(*types.ArrayType); isSourceArray {
							if sourceArrayType.ElementType == types.Unknown {
								isEmptyArrayAssignment = true
							}
						}
					}

					if !assignable && !isEmptyArrayAssignment {
						// Check if this is an enum assignment to use appropriate error format
						if c.isEnumType(finalVariableType) {
							// For enum assignments, use widened source type and no variable name
							sourceTypeStr, targetTypeStr := c.getEnumAssignmentErrorTypes(computedInitializerType, finalVariableType)
							c.addError(declarator.Value, fmt.Sprintf("cannot assign type '%s' to variable of type '%s'", sourceTypeStr, targetTypeStr))
						} else {
							// For regular variable assignments, use literal types and include variable name
							sourceTypeStr, targetTypeStr := c.getAssignmentErrorTypes(computedInitializerType, finalVariableType)
							variableName := nameValueStr
							c.addError(declarator.Value, fmt.Sprintf("cannot assign type '%s' to variable '%s' of type '%s'", sourceTypeStr, variableName, targetTypeStr))
						}
					}
				}
			} else {
				// --- No annotation: Infer type ---
				if computedInitializerType != nil {
					if _, isLiteral := computedInitializerType.(*types.LiteralType); isLiteral {
						finalVariableType = types.GetWidenedType(computedInitializerType)
						debugPrintf("// [Checker VarStmt] '%s': Inferred final type (widened literal): %s (Go Type: %T)\n", nameValueStr, finalVariableType.String(), finalVariableType)
					} else {
						finalVariableType = computedInitializerType
						debugPrintf("// [Checker VarStmt] '%s': Assigned finalVariableType (direct non-literal): %s (Go Type: %T)\n", nameValueStr, finalVariableType.String(), finalVariableType)
					}
				} else {
					finalVariableType = types.Undefined
					debugPrintf("// [Checker VarStmt] '%s': Inferred final type (no initializer): %s (Go Type: %T)\n", nameValueStr, finalVariableType.String(), finalVariableType)
				}
			}

			// 4. UPDATE variable type in function scope with the final type
			currentType, _, _ := funcScope.Resolve(declarator.Name.Value)
			debugPrintf("// [Checker VarStmt] Updating '%s'. Current type: %s, Final type: %s\n", declarator.Name.Value, currentType.String(), finalVariableType.String())
			if !funcScope.Update(declarator.Name.Value, finalVariableType) {
				debugPrintf("// [Checker VarStmt] WARNING: Update failed unexpectedly for '%s'\n", declarator.Name.Value)
			}
			updatedType, _, _ := funcScope.Resolve(declarator.Name.Value)
			debugPrintf("// [Checker VarStmt] Updated '%s'. Type after update: %s\n", declarator.Name.Value, updatedType.String())

			// Set computed type on the Name Identifier node itself and the declarator
			declarator.Name.SetComputedType(finalVariableType)
			declarator.ComputedType = finalVariableType
		}

	case *parser.ArrayDestructuringDeclaration:
		c.checkArrayDestructuringDeclaration(node)

	case *parser.ObjectDestructuringDeclaration:
		c.checkObjectDestructuringDeclaration(node)

	case *parser.ReturnStatement:
		// --- UPDATED: Handle ReturnStatement ---
		var actualReturnType types.Type = types.Undefined // Default if no return value
		if node.ReturnValue != nil {
			// Use contextual typing if we have an expected return type
			// This allows array literals to be typed as tuples when expected
			if c.currentExpectedReturnType != nil {
				c.visitWithContext(node.ReturnValue, &ContextualType{
					ExpectedType: c.currentExpectedReturnType,
				})
			} else {
				c.visit(node.ReturnValue)
			}
			actualReturnType = node.ReturnValue.GetComputedType() // <<< USE NODE METHOD
			if actualReturnType == nil {
				actualReturnType = types.Any
			} // Handle nil from potential error
		}

		// Check against expected type if available
		if c.currentExpectedReturnType != nil {
			debugPrintf("// [Checker Return] Checking return type: actual=%T(%s) vs expected=%T(%s)\n",
				actualReturnType, actualReturnType.String(), c.currentExpectedReturnType, c.currentExpectedReturnType.String())

			// Debug type parameter instances
			if actualTPT, ok := actualReturnType.(*types.TypeParameterType); ok {
				if expectedTPT, ok := c.currentExpectedReturnType.(*types.TypeParameterType); ok {
					debugPrintf("// [Checker Return] Type parameter comparison: actual.Parameter=%p vs expected.Parameter=%p\n",
						actualTPT.Parameter, expectedTPT.Parameter)
					debugPrintf("// [Checker Return] Type parameter names: actual=%s vs expected=%s\n",
						actualTPT.Parameter.Name, expectedTPT.Parameter.Name)
				}
			}

			// Special handling for type predicate return types
			if _, ok := c.currentExpectedReturnType.(*types.TypePredicateType); ok {
				// Type predicate functions should accept boolean returns
				if !types.IsAssignable(actualReturnType, types.Boolean) {
					msg := fmt.Sprintf("cannot return value of type %s from type predicate function expecting boolean",
						actualReturnType)
					c.addError(node.ReturnValue, msg)
				}
			} else if !types.IsAssignable(actualReturnType, c.currentExpectedReturnType) {
				// Debug: check pointer addresses
				fmt.Printf("DEBUG Return check: actualType=%T(%p) expectedType=%T(%p)\n",
					actualReturnType, actualReturnType, c.currentExpectedReturnType, c.currentExpectedReturnType)
				msg := fmt.Sprintf("cannot return value of type %s from function expecting %s",
					actualReturnType, c.currentExpectedReturnType)
				// Report the error at the return value expression node
				c.addError(node.ReturnValue, msg)
			}
		}

		// Add to inferred types - widen literal types for better inference
		// This is especially important for generators where the return type becomes TReturn parameter
		widenedReturnType := types.GetWidenedType(actualReturnType)
		c.currentInferredReturnTypes = append(c.currentInferredReturnTypes, widenedReturnType)

	case *parser.BlockStatement:
		// --- UPDATED: Handle Block Scope & Hoisting ---
		debugPrintf("// [Checker Visit Block] Entering Block. Current Env: %p\n", c.env)
		if c.env == nil {
			panic("Checker env is nil before creating block scope!")
		}
		// 1. Create a new enclosed environment
		originalEnv := c.env // Store the current environment
		c.env = NewEnclosedEnvironment(originalEnv)
		debugPrintf("// [Checker Visit Block] Created Block Env: %p (outer: %p)\n", c.env, originalEnv) // DEBUG

		// --- NEW: Process Hoisted Declarations for this block FIRST ---
		if node.HoistedDeclarations != nil {
			for name, hoistedNode := range node.HoistedDeclarations {
				funcLit, ok := hoistedNode.(*parser.FunctionLiteral)
				if !ok {
					debugPrintf("// [Checker Block Hoisting] ERROR: Hoisted node for '%s' is not a FunctionLiteral (%T)\n", name, hoistedNode)
					continue
				}

				// Resolve the function type signature using the *current* (block) environment
				funcSig := c.resolveFunctionLiteralSignature(funcLit, c.env)
				if funcSig == nil {
					debugPrintf("// [Checker Block Hoisting] WARNING: Failed to resolve signature for hoisted func '%s'. Defining as Any.\n", name)
					if !c.env.Define(name, types.Any, false) {
						c.addError(funcLit.Name, fmt.Sprintf("identifier '%s' already defined in this block scope", name))
					}
					continue
				}

				// Convert signature to ObjectType and define the function in the block environment
				funcObjectType := types.NewFunctionType(funcSig)
				if !c.env.Define(name, funcObjectType, false) {
					// Duplicate definition error
					c.addError(funcLit.Name, fmt.Sprintf("identifier '%s' already defined in this block scope", name))
				}

				// Set the computed type on the FunctionLiteral node itself NOW.
				funcLit.SetComputedType(funcObjectType)
				debugPrintf("// [Checker Block Hoisting] Hoisted and defined func '%s' with type: %s\n", name, funcObjectType.String())
			}
		}
		// --- END Block Hoisted Declarations Processing ---

		// 2. Visit statements within the new scope
		if node.Statements == nil {
			debugPrintf("// [Checker Visit Block] WARNING: node.Statements is nil for Block %p\n", node)
		} else {
			debugPrintf("// [Checker Visit Block] node.Statements length: %d\n", len(node.Statements))
		}
		// --- END DEBUG ---

		// 3. Visit statements within the new scope
		for i, stmt := range node.Statements { // Add index 'i' for logging
			// --- DEBUG ---
			debugPrintf("// [Checker Visit Block Loop] Index: %d, Stmt Type: %T, Stmt Ptr: %p\n", i, stmt, stmt)
			if stmt == nil {
				debugPrintf("// [Checker Visit Block Loop] ERROR: Stmt at index %d is nil! Skipping.\n", i)
				continue // Skip visiting nil statement
			}
			// --- END DEBUG ---
			c.visit(stmt)
		}

		// --- DEBUG ---
		debugPrintf("// [Checker Visit Block] Exiting Block. Restoring Env: %p (from current %p)\n", originalEnv, c.env)
		if originalEnv == nil {
			panic("Checker originalEnv is nil before restoring block scope!")
		}
		// --- END DEBUG ---
		// 4. Restore the outer environment
		c.env = originalEnv

	// --- Literal Expressions ---
	case *parser.NumberLiteral:
		literalType := &types.LiteralType{Value: vm.Number(node.Value)}
		node.SetComputedType(literalType) // <<< USE NODE METHOD

	case *parser.BigIntLiteral:
		// Parse the numeric part and create a big.Int
		bigIntValue := new(big.Int)
		if _, ok := bigIntValue.SetString(node.Value, 0); !ok {
			c.addError(node, fmt.Sprintf("Invalid BigInt literal: %s", node.Value))
			node.SetComputedType(types.Never)
		} else {
			literalType := &types.LiteralType{Value: vm.NewBigInt(bigIntValue)}
			node.SetComputedType(literalType)
		}

	case *parser.StringLiteral:
		literalType := &types.LiteralType{Value: vm.String(node.Value)}
		node.SetComputedType(literalType) // <<< USE NODE METHOD

	case *parser.RegexLiteral:
		// For now, treat regex literals as the general RegExp type
		// Later we can add more specific typing based on patterns/flags
		node.SetComputedType(types.RegExp)

	case *parser.BooleanLiteral:
		literalType := &types.LiteralType{Value: vm.BooleanValue(node.Value)}
		// Treat boolean literals as literal types during checking
		node.SetComputedType(literalType) // <<< USE NODE METHOD

	case *parser.NullLiteral:
		node.SetComputedType(types.Null)

	case *parser.UndefinedLiteral:
		node.SetComputedType(types.Undefined)

	// --- NEW: Handle TemplateLiteral ---
	case *parser.TemplateLiteral:
		c.checkTemplateLiteral(node)
	case *parser.TaggedTemplateExpression:
		c.checkTaggedTemplateExpression(node)

	// --- NEW: Handle ThisExpression ---
	case *parser.ThisExpression:
		// In global context or regular function context, 'this' is undefined
		// In method context, 'this' refers to the object the method is called on
		if c.currentThisType != nil {
			node.SetComputedType(c.currentThisType)
			debugPrintf("// [Checker ThisExpr] Using context this type: %s\n", c.currentThisType.String())
		} else {
			// Global context or regular function - 'this' is undefined
			node.SetComputedType(types.Undefined)
			debugPrintf("// [Checker ThisExpr] No this context, using undefined\n")
		}

	case *parser.SuperExpression:
		// Super expression handling - must be in a class context with inheritance
		c.checkSuperExpression(node)

	case *parser.NewTargetExpression:
		// new.target is only valid in functions that can be called as constructors
		// For now, we'll type it as undefined in all contexts
		// TODO: Properly track constructor context and return the constructor function type
		node.SetComputedType(types.Undefined)
		debugPrintf("// [Checker NewTargetExpr] Typed as undefined (TODO: implement proper constructor context)\n")

	case *parser.ImportMetaExpression:
		// import.meta is an object containing module metadata
		// For now, we'll type it as any
		// TODO: Create proper ImportMeta type with url property etc.
		node.SetComputedType(types.Any)
		debugPrintf("// [Checker ImportMetaExpr] Typed as any (TODO: implement proper ImportMeta type)\n")

	case *parser.DynamicImportExpression:
		// Dynamic import returns Promise<Module>
		// Check that the source expression is a string (or can be coerced to string)
		c.visit(node.Source)
		sourceType := node.Source.GetComputedType()

		// The specifier should be a string or any type that can be converted to string
		// For simplicity, accept any type (JavaScript allows this)
		_ = sourceType

		// Return type is Promise<any> for now
		// TODO: Create proper Promise<Module> type with module namespace exports
		node.SetComputedType(types.Any) // Should be Promise<ModuleNamespace>
		debugPrintf("// [Checker DynamicImportExpr] Typed as Promise<any> (TODO: implement proper Promise<Module> type)\n")

	case *parser.DeferredImportExpression:
		// Deferred import (import.defer) returns Promise<DeferredModule>
		// Check that the source expression is a string (or can be coerced to string)
		c.visit(node.Source)
		sourceType := node.Source.GetComputedType()

		// The specifier should be a string or any type that can be converted to string
		// For simplicity, accept any type (JavaScript allows this)
		_ = sourceType

		// Return type is Promise<any> for now (same as dynamic import)
		// TODO: Create proper Promise<DeferredModule> type
		node.SetComputedType(types.Any) // Should be Promise<DeferredModuleNamespace>
		debugPrintf("// [Checker DeferredImportExpr] Typed as Promise<any> (TODO: implement proper Promise<DeferredModule> type)\n")

	// --- Other Expressions ---
	case *parser.Identifier:
		// --- Check concrete pointer AFTER type switch ---
		if node == nil {
			debugPrintf("// [Checker Debug] visit(Identifier): node is nil!\n") // DEBUG
			return
		}
		// --- Log state BEFORE potentially problematic operations ---
		debugPrintf("// [Checker Debug] visit(Identifier): node=%p, Value='%s', Token={%s %q %d}\n", node, node.Value, node.Token.Type, node.Token.Literal, node.Token.Line) // DEBUG
		debugPrintf("// [Checker Debug] visit(Identifier): c.env=%p\n", c.env)                                                                                               // DEBUG

		// --- UPDATED: Handle Identifier (Value Context Only) ---
		// Assume this is visited in a value context.
		// Type context identifiers are handled by resolveTypeAnnotation.

		// Safety check for incomplete nodes from parser errors - *REMOVED* (covered by check above)
		// if node == nil {
		// 	return
		// }

		// First try regular resolution, then check with objects
		typ, isConst, found := c.env.Resolve(node.Value) // Use node.Value directly; UPDATED TO 3 VARS
		isFromWith := false

		// If not found as a regular variable, try with object resolution
		if !found {
			typ, isFromWith, found = c.env.ResolveWithFallback(node.Value)
			isConst = false // Properties from with objects are never const
		}

		if !found {
			// Special handling for 'arguments' identifier - only available in function scope
			if node.Value == "arguments" {
				// Check if we're inside a function (not global scope)
				if c.env.outer != nil { // Function scope has an outer environment
					// Get IArguments type that was defined by the builtin initializer
					// Walk up to the global environment to find IArguments
					globalEnv := c.env
					for globalEnv.outer != nil {
						globalEnv = globalEnv.outer
					}
					iArgumentsType, _, hasIArguments := globalEnv.Resolve("IArguments")
					if hasIArguments {
						debugPrintf("// [Checker Debug] visit(Identifier): 'arguments' resolved as IArguments type in function scope\n")
						node.SetComputedType(iArgumentsType)
						node.IsConstant = false // arguments is not a constant
						return                  // Successfully handled 'arguments' identifier
					}
				}
				// If in global scope or IArguments type not found, fall through to error
			}

			// Check for built-in function before reporting error
			builtinType := c.getBuiltinType(node.Value)
			if builtinType != nil {
				debugPrintf("// [Checker Debug] visit(Identifier): '%s' resolved as built-in type: %s\n", node.Value, builtinType.String())
				node.SetComputedType(builtinType)
				node.IsConstant = true // Built-ins are effectively constants
			} else if c.allowForwardReferences {
				// In lenient mode, treat undefined variables as 'any' instead of errors
				// This allows checking method bodies that reference variables declared later
				debugPrintf("// [Checker Debug] visit(Identifier): '%s' not found (forward reference allowed)\n", node.Value)
				node.SetComputedType(types.Any)
			} else {
				debugPrintf("// [Checker Debug] visit(Identifier): '%s' not found in env %p\n", node.Value, c.env) // DEBUG
				c.addError(node, fmt.Sprintf("undefined variable: %s", node.Value))
				// Set computed type if node itself is not nil (already checked)
				node.SetComputedType(types.Any) // Set to Any on error?
			}
		} else {
			// --- DEBUG: Log raw type value immediately after resolve ---
			// debugPrintf("// [Checker Debug] Identifier: Resolved type for '%s': Ptr=%p, Value=%#v\n", node.Value, typ, typ) // Re-commented
			// --- END DEBUG ---

			if typ != nil {
				debugPrintf("// [Checker Debug] visit(Identifier): '%s' found in env %p, type: %s\n", node.Value, c.env, typ.String()) // DEBUG - Uncommented
			} else {
				debugPrintf("// [Checker Debug] visit(Identifier): '%s' found in env %p, type: <nil>\n", node.Value, c.env) // DEBUG - Nil type
			}

			// node is guaranteed non-nil here
			node.SetComputedType(typ)
			// --- DEBUG: Explicit panic before return ---
			// panic(fmt.Sprintf("Intentional panic after setting type for Identifier '%s'", node.Value))
			// --- END DEBUG ---

			// <<< ADD CONST CHECK FOR IDENTIFIER NODE >>>
			// Store the const status on the identifier node itself for later use in assignment checks.
			node.IsConstant = isConst // Re-enabled
			// Store whether this identifier came from a with object (for compiler)
			node.IsFromWith = isFromWith
		}

	case *parser.PrefixExpression:
		// --- UPDATED: Handle PrefixExpression ---
		// Special case: delete operator with identifier - don't throw error for undefined identifiers
		// Similar to typeof, delete on unresolvable reference should work in non-strict mode
		if node.Operator == "delete" {
			if ident, ok := node.Right.(*parser.Identifier); ok {
				// Check if identifier exists
				_, _, found := c.env.Resolve(ident.Value)
				if !found {
					// Identifier doesn't exist - this is valid for delete (returns true)
					// Set computed type and return early
					node.SetComputedType(types.Boolean)
					return
				}
			}
		}
		c.visit(node.Right) // Visit the operand first
		rightType := node.Right.GetComputedType()
		var resultType types.Type = types.Any // Default to Any on error

		if rightType != nil { // Proceed only if operand type is known
			widenedRightType := types.GetWidenedType(rightType)
			switch node.Operator {
			case "-":
				if widenedRightType == types.Any {
					resultType = types.Any
				} else if widenedRightType == types.Number {
					resultType = types.Number
				} else {
					c.addError(node, fmt.Sprintf("operator '%s' cannot be applied to type '%s'", node.Operator, widenedRightType.String()))
					// Keep resultType = types.Any (default)
				}
			case "!":
				resultType = types.Boolean
			// --- NEW: Handle Unary Plus (+) ---
			case "+":
				// Unary plus coerces operand to number
				if widenedRightType == types.Any {
					resultType = types.Any
				} else {
					// In TypeScript/JavaScript, unary plus always attempts to convert to number
					// For type checking purposes, the result is always number (even if it could be NaN at runtime)
					resultType = types.Number
				}
			// --- NEW: Handle Void ---
			case "void":
				// void operator always returns undefined, regardless of operand type
				// The operand is still evaluated (for side effects), but the result is always undefined
				resultType = types.Undefined
			// --- NEW: Handle Bitwise NOT (~) ---
			case "~":
				if widenedRightType == types.Any {
					resultType = types.Any // Bitwise NOT on 'any' results in 'any'? Or number? Let's stick with number like other bitwise ops.
					resultType = types.Number
				} else if widenedRightType == types.Number {
					resultType = types.Number // Result of ~number is number
				} else {
					c.addError(node, fmt.Sprintf("operator '%s' cannot be applied to type '%s'", node.Operator, widenedRightType.String()))
					// Keep resultType = types.Any (default)
				}
			// --- NEW: Handle delete operator ---
			case "delete":
				// delete operator returns boolean indicating success
				resultType = types.Boolean
				// Per ECMAScript spec:
				// - delete <property-access> returns true/false based on deletion success
				// - delete <identifier> returns true (strict mode would throw, but we allow it)
				// - delete <non-reference> (literal, expression) returns true
				// Only check for readonly properties on member expressions
				switch rightNode := node.Right.(type) {
				case *parser.MemberExpression:
					// Check if the property is readonly
					if objType := rightNode.Object.GetComputedType(); objType != nil {
						if objTypeResolved, ok := objType.(*types.ObjectType); ok {
							propName := ""
							if ident, ok := rightNode.Property.(*parser.Identifier); ok {
								propName = ident.Value
							}
							if propName != "" {
								if _, exists := objTypeResolved.Properties[propName]; exists {
									// Check if property is readonly (for built-in types like Math)
									if objTypeResolved.IsReadOnly(propName) {
										c.addError(node, "The operand of a 'delete' operator cannot be a read-only property.")
									}
								}
							}
						}
					}
				case *parser.IndexExpression:
					// Valid delete target (bracket notation)
				case *parser.Identifier:
					// Allow delete on identifiers (returns true in non-strict mode, throws in strict)
					// We're lenient here for JavaScript compatibility
				default:
					// All other expressions are allowed (literals, typeof, arithmetic, etc.)
					// They all return true per ECMAScript spec
				}
			// --- END NEW ---
			default:
				c.addError(node, fmt.Sprintf("unsupported prefix operator: %s", node.Operator))
			}
		} // else: Error might have occurred visiting operand, or type is nil.
		node.SetComputedType(resultType)

	case *parser.TypeofExpression:
		// --- UPDATED: Handle TypeofExpression ---
		c.checkTypeofExpression(node)

	case *parser.TypeAssertionExpression:
		// --- NEW: Handle TypeAssertionExpression ---
		c.checkTypeAssertionExpression(node)

	case *parser.SatisfiesExpression:
		// --- NEW: Handle SatisfiesExpression ---
		c.checkSatisfiesExpression(node)

	case *parser.NonNullExpression:
		// Handle non-null assertion expression (x!)
		c.checkNonNullExpression(node)

	case *parser.InfixExpression:
		// --- UPDATED: Handle InfixExpression ---
		c.visit(node.Left)
		c.visit(node.Right)
		leftType := node.Left.GetComputedType()

		if leftType == nil {
			leftType = types.Any
		}
		rightType := node.Right.GetComputedType()

		if rightType == nil {
			rightType = types.Any
		}

		widenedLeftType := types.GetWidenedType(leftType)
		widenedRightType := types.GetWidenedType(rightType)

		debugPrintf("// [Checker Infix Pre-Check] Left : %T (%v)\n", leftType, leftType)
		debugPrintf("// [Checker Infix Pre-Check] Right: %T (%v)\n", rightType, rightType)
		debugPrintf("// [Checker Infix Pre-Check] Widened Left : %T (%v)\n", widenedLeftType, widenedLeftType)
		debugPrintf("// [Checker Infix Pre-Check] Widened Right: %T (%v)\n", widenedRightType, widenedRightType)
		debugPrintf("// [Checker Infix Pre-Check] Check Condition: %v\n", widenedLeftType != nil && widenedRightType != nil)

		var resultType types.Type = types.Any // Default to Any on error
		isAnyOperand := widenedLeftType == types.Any || widenedRightType == types.Any

		if widenedLeftType != nil && widenedRightType != nil {
			debugPrintf("// [Checker Infix Pre-Check] Proceeding with operator: %s\n", node.Operator)
			switch node.Operator {
			case "+":
				if isAnyOperand {
					resultType = types.Any
				} else if widenedLeftType == types.Number && widenedRightType == types.Number {
					resultType = types.Number
				} else if types.IsEnumMemberType(leftType) && types.IsEnumMemberType(rightType) {
					// Check if both enum members are numeric
					leftValue, _ := types.GetEnumMemberValue(leftType)
					rightValue, _ := types.GetEnumMemberValue(rightType)
					if _, leftIsNumeric := leftValue.(int); leftIsNumeric {
						if _, rightIsNumeric := rightValue.(int); rightIsNumeric {
							resultType = types.Number
						} else {
							c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
						}
					} else {
						c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
					}
				} else if types.IsEnumMemberType(leftType) && widenedRightType == types.Number {
					// Enum member + number
					leftValue, _ := types.GetEnumMemberValue(leftType)
					if _, leftIsNumeric := leftValue.(int); leftIsNumeric {
						resultType = types.Number
					} else {
						c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
					}
				} else if widenedLeftType == types.Number && types.IsEnumMemberType(rightType) {
					// Number + enum member
					rightValue, _ := types.GetEnumMemberValue(rightType)
					if _, rightIsNumeric := rightValue.(int); rightIsNumeric {
						resultType = types.Number
					} else {
						c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
					}
				} else if widenedLeftType == types.BigInt && widenedRightType == types.BigInt {
					resultType = types.BigInt
				} else if (widenedLeftType == types.BigInt && widenedRightType == types.Number) ||
					(widenedLeftType == types.Number && widenedRightType == types.BigInt) {
					c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s' (cannot mix BigInt and other types)", node.Operator, widenedLeftType.String(), widenedRightType.String()))
					// Keep resultType = types.Any (default)
				} else if widenedLeftType == types.String && widenedRightType == types.String {
					resultType = types.String
					// <<< NEW: Handle String + Number/BigInt Coercion >>>
				} else if (widenedLeftType == types.String && widenedRightType == types.Number) ||
					(widenedLeftType == types.Number && widenedRightType == types.String) {
					resultType = types.String
				} else if (widenedLeftType == types.String && widenedRightType == types.BigInt) ||
					(widenedLeftType == types.BigInt && widenedRightType == types.String) {
					resultType = types.String
				} else if (widenedLeftType == types.String && widenedRightType == types.Boolean) ||
					(widenedLeftType == types.Boolean && widenedRightType == types.String) {
					resultType = types.String
				} else if (widenedLeftType == types.String && c.isStringConcatenatable(rightType)) ||
					(c.isStringConcatenatable(leftType) && widenedRightType == types.String) {
					// TypeScript allows string concatenation with most types (including unions)
					resultType = types.String
				} else if (widenedLeftType == types.Boolean || widenedLeftType == types.Null || widenedLeftType == types.Undefined) &&
					(widenedRightType == types.Boolean || widenedRightType == types.Null || widenedRightType == types.Undefined || widenedRightType == types.Number) {
					// JavaScript allows boolean/null/undefined in addition, they're coerced to numbers
					// true → 1, false → 0, null → 0, undefined → NaN
					resultType = types.Number
				} else if (widenedRightType == types.Boolean || widenedRightType == types.Null || widenedRightType == types.Undefined) &&
					(widenedLeftType == types.Number) {
					// Number + boolean/null/undefined → number
					resultType = types.Number
				} else if c.isObjectType(widenedLeftType) || c.isObjectType(widenedRightType) {
					// JavaScript allows objects in addition via ToPrimitive conversion
					// Object + anything or anything + Object → depends on ToPrimitive result
					// If ToPrimitive returns string, result is string; otherwise number
					// Conservative: assume string since that's most common for objects
					resultType = types.String
				} else {
					c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, widenedLeftType.String(), widenedRightType.String()))
					// Keep resultType = types.Any (default)
				}
			case "-", "*", "/":
				if isAnyOperand {
					resultType = types.Any
				} else if widenedLeftType == types.Number && widenedRightType == types.Number {
					resultType = types.Number
				} else if types.IsEnumMemberType(leftType) && types.IsEnumMemberType(rightType) {
					// Check if both enum members are numeric
					leftValue, _ := types.GetEnumMemberValue(leftType)
					rightValue, _ := types.GetEnumMemberValue(rightType)
					if _, leftIsNumeric := leftValue.(int); leftIsNumeric {
						if _, rightIsNumeric := rightValue.(int); rightIsNumeric {
							resultType = types.Number
						} else {
							c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
						}
					} else {
						c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
					}
				} else if types.IsEnumMemberType(leftType) && widenedRightType == types.Number {
					// Enum member with number
					leftValue, _ := types.GetEnumMemberValue(leftType)
					if _, leftIsNumeric := leftValue.(int); leftIsNumeric {
						resultType = types.Number
					} else {
						c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
					}
				} else if widenedLeftType == types.Number && types.IsEnumMemberType(rightType) {
					// Number with enum member
					rightValue, _ := types.GetEnumMemberValue(rightType)
					if _, rightIsNumeric := rightValue.(int); rightIsNumeric {
						resultType = types.Number
					} else {
						c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
					}
				} else if widenedLeftType == types.BigInt && widenedRightType == types.BigInt {
					resultType = types.BigInt
				} else if (widenedLeftType == types.BigInt && widenedRightType == types.Number) ||
					(widenedLeftType == types.Number && widenedRightType == types.BigInt) {
					c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s' (cannot mix BigInt and other types)", node.Operator, widenedLeftType.String(), widenedRightType.String()))
					// Keep resultType = types.Any (default)
				} else if (widenedLeftType == types.String && widenedRightType == types.Number) ||
					(widenedLeftType == types.Number && widenedRightType == types.String) {
					// JavaScript allows string-number arithmetic, resulting in NaN
					resultType = types.Number
				} else if (widenedLeftType == types.String && widenedRightType == types.BigInt) ||
					(widenedLeftType == types.BigInt && widenedRightType == types.String) {
					// JavaScript allows string-BigInt arithmetic, resulting in NaN
					resultType = types.BigInt
				} else {
					c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, widenedLeftType.String(), widenedRightType.String()))
					// Keep resultType = types.Any (default)
				}
			// --- NEW: Handle % and ** type checking ---
			case "%", "**": // Ensure both % and ** are listed here
				debugPrintf("// [Checker Infix Pre-Check] Proceeding with operator: %s\n", node.Operator)
				if isAnyOperand {
					resultType = types.Any
				} else if widenedLeftType == types.Number && widenedRightType == types.Number {
					resultType = types.Number
				} else if types.IsEnumMemberType(leftType) && types.IsEnumMemberType(rightType) {
					// Check if both enum members are numeric
					leftValue, _ := types.GetEnumMemberValue(leftType)
					rightValue, _ := types.GetEnumMemberValue(rightType)
					if _, leftIsNumeric := leftValue.(int); leftIsNumeric {
						if _, rightIsNumeric := rightValue.(int); rightIsNumeric {
							resultType = types.Number
						} else {
							c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
						}
					} else {
						c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
					}
				} else if types.IsEnumMemberType(leftType) && widenedRightType == types.Number {
					// Enum member with number
					leftValue, _ := types.GetEnumMemberValue(leftType)
					if _, leftIsNumeric := leftValue.(int); leftIsNumeric {
						resultType = types.Number
					} else {
						c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
					}
				} else if widenedLeftType == types.Number && types.IsEnumMemberType(rightType) {
					// Number with enum member
					rightValue, _ := types.GetEnumMemberValue(rightType)
					if _, rightIsNumeric := rightValue.(int); rightIsNumeric {
						resultType = types.Number
					} else {
						c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
					}
				} else if widenedLeftType == types.BigInt && widenedRightType == types.BigInt {
					resultType = types.BigInt
				} else if (widenedLeftType == types.BigInt && widenedRightType == types.Number) ||
					(widenedLeftType == types.Number && widenedRightType == types.BigInt) {
					c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s' (cannot mix BigInt and other types)", node.Operator, widenedLeftType.String(), widenedRightType.String()))
					// Keep resultType = types.Any (default)
				} else {
					c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, widenedLeftType.String(), widenedRightType.String()))
					// Keep resultType = types.Any
				}
			// --- END NEW ---

			// --- NEW: Handle Bitwise/Shift Operators ---
			case "&", "|", "^", "<<", ">>", ">>>":
				if isAnyOperand {
					// If either operand is 'any', the result is likely 'number'
					// as these ops coerce non-numbers in JS (often to 0 or NaN).
					// Let's assume 'number' is the most probable outcome type.
					resultType = types.Number
				} else if widenedLeftType == types.Number && widenedRightType == types.Number {
					// Both operands are numbers, result is number.
					resultType = types.Number
				} else if widenedLeftType == types.BigInt && widenedRightType == types.BigInt {
					// Both operands are BigInt, result is BigInt.
					resultType = types.BigInt
				} else if (widenedLeftType == types.BigInt && widenedRightType == types.Number) ||
					(widenedLeftType == types.Number && widenedRightType == types.BigInt) {
					// Mixing BigInt and Number is not allowed for bitwise/shift operations
					c.addError(node, fmt.Sprintf("operator '%s' cannot mix BigInt and Number types", node.Operator))
					// Keep resultType = types.Any (default)
				} else {
					// Check if either operand is an object type (can be converted via valueOf/toString)
					leftIsObject := false
					rightIsObject := false

					switch widenedLeftType.(type) {
					case *types.ObjectType:
						leftIsObject = true
					}

					switch widenedRightType.(type) {
					case *types.ObjectType:
						rightIsObject = true
					}

					if leftIsObject && rightIsObject {
						// Both operands are objects (can be converted via valueOf/toString)
						resultType = types.Number
					} else if leftIsObject && widenedRightType == types.Number {
						// Left operand is object, right is number
						resultType = types.Number
					} else if widenedLeftType == types.Number && rightIsObject {
						// Left operand is number, right is object
						resultType = types.Number
					} else {
						// Operands are not compatible types for bitwise/shift operations.
						c.addError(node, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, widenedLeftType.String(), widenedRightType.String()))
						// Keep resultType = types.Any (default)
					}
				}
			// --- END NEW ---

			case "<", ">", "<=", ">=":
				if isAnyOperand {
					resultType = types.Any // Comparison with any results in any? Or boolean? Let's try Any first.
					// Alternatively: resultType = types.Boolean (safer, result is always boolean)
				} else if widenedLeftType == types.Number && widenedRightType == types.Number {
					resultType = types.Boolean
				} else if widenedLeftType == types.String && widenedRightType == types.String {
					resultType = types.Boolean
				} else {
					c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, widenedLeftType.String(), widenedRightType.String()))
					resultType = types.Boolean // Comparison errors still result in boolean
				}
			case "==", "!=", "===", "!==":
				// Check for impossible comparisons before setting result type
				c.checkImpossibleComparison(leftType, rightType, node.Operator, node)
				// Comparison always results in boolean, even with 'any'
				resultType = types.Boolean
			case "in":
				// Property existence check: "prop" in obj
				c.checkInOperator(leftType, rightType, node)
				resultType = types.Boolean
			case "instanceof":
				// Instance check: obj instanceof Constructor
				c.checkInstanceofOperator(leftType, rightType, node)
				resultType = types.Boolean
			case "&&", "||":
				// TODO: Implement Union types. For now, default to Any.
				// If one operand is any, result is any.
				if isAnyOperand {
					resultType = types.Any
				} else {
					// Need proper type analysis here based on logic.
					// For now, fallback to Any if not involving Any.
					resultType = types.Any
				}
			case "??":
				// TODO: Implement Union types. For now, default to Any.
				// If left is any, result is any. If right is any and left is null/undef, result is any.
				if isAnyOperand { // Simplified check for now
					resultType = types.Any
				} else {
					// Need proper type analysis here based on null/undefined checks.
					// For now, fallback to Any if not involving Any.
					resultType = types.Any
				}
			case ",":
				// Comma operator: evaluates both expressions but returns the type of the right expression
				// (left, right) -> right_type
				resultType = rightType
				debugPrintf("// [Checker Comma] Left evaluated but discarded, result type: %s\n", rightType.String())
			default:
				debugPrintf("// [Checker Infix Pre-Check] Proceeding with operator: %s\n", node.Operator)
				c.addError(node.Right, fmt.Sprintf("unsupported infix operator: %s", node.Operator))
			}
		} // else: Error already reported during operand check or types were nil

		debugPrintf("// [Checker Infix] Node: %p (%s), Determined ResultType: %T (%v)\n", node, node.Operator, resultType, resultType)
		node.SetComputedType(resultType)

	case *parser.IfExpression:
		// --- UPDATED: Handle IfExpression with Type Narrowing ---
		// 1. Check Condition
		c.visit(node.Condition)

		// 2. Check Consequence block with type narrowing (supports compound conditions)
		originalEnv := c.env
		narrowedEnv := c.applyTypeNarrowingWithFallback(node.Condition)

		if narrowedEnv != nil {
			debugPrintf("// [Checker IfExpr] Applying type narrowing in consequence block\n")
			c.env = narrowedEnv // Use narrowed environment for consequence
		}

		c.visit(node.Consequence)

		// Restore original environment before checking alternative
		c.env = originalEnv

		// 4. Check Alternative block (if it exists)
		if node.Alternative != nil {
			// For compound conditions, inverted narrowing is complex, so skip for now
			// TODO: Implement inverted narrowing for compound conditions

			c.visit(node.Alternative)

			// Restore original environment after alternative
			c.env = originalEnv
		}

		// 5. Determine overall type (tricky! depends on return/break/continue)
		// For now, if expressions don't have a value themselves (unless ternary)
		// They control flow. Let's assign Void for now, representing no value produced by the if itself.
		// A more advanced checker might determine if both branches *must* return/throw,
		// or compute a union type of the last expressions if they are treated as values.
		node.SetComputedType(types.Void) // Use checker's method

	case *parser.IfStatement:
		// --- NEW: Handle IfStatement with Type Narrowing ---
		// Similar to IfExpression but for statement context
		// 1. Check Condition
		c.visit(node.Condition)

		// 2. Check Consequence block with type narrowing (supports compound conditions)
		originalEnv := c.env
		narrowedEnv := c.applyTypeNarrowingWithFallback(node.Condition)

		if narrowedEnv != nil {
			debugPrintf("// [Checker IfStmt] Applying type narrowing in consequence block\n")
			c.env = narrowedEnv // Use narrowed environment for consequence
		}

		c.visit(node.Consequence)

		// Restore original environment before checking alternative
		c.env = originalEnv

		// Detect type guard for inverted narrowing
		typeGuard := c.detectTypeGuard(node.Condition)

		// 4. Check Alternative block (if it exists) with inverted narrowing
		if node.Alternative != nil {
			var invertedEnv *Environment
			if typeGuard != nil {
				invertedEnv = c.applyInvertedTypeNarrowing(typeGuard)
			}

			if invertedEnv != nil {
				debugPrintf("// [Checker IfStmt] Applying inverted type narrowing in alternative block\n")
				c.env = invertedEnv // Use inverted narrowed environment for alternative
			}

			c.visit(node.Alternative)

			// Restore original environment
			c.env = originalEnv
		}

		// 5. Control flow narrowing: if consequence block always terminates (returns, throws, etc.)
		// then apply inverted narrowing for the code after the if statement
		consequenceTerminates := blockAlwaysTerminates(node.Consequence)
		if consequenceTerminates && node.Alternative == nil {
			// The if block terminates, so code after only runs when condition was false
			if typeGuard != nil {
				invertedEnv := c.applyInvertedTypeNarrowing(typeGuard)
				if invertedEnv != nil {
					debugPrintf("// [Checker IfStmt] Consequence terminates, applying inverted narrowing after if\n")
					c.env = invertedEnv
				}
			}
		} else {
			// Restore original environment after if statement
			c.env = originalEnv
		}

		// 6. IfStatement doesn't have a value/type (it's a statement, not expression)

	case *parser.TernaryExpression:
		// --- UPDATED: Handle TernaryExpression with Type Narrowing ---
		// 1. Check Condition
		c.visit(node.Condition)

		// 2. Detect type guards in the condition
		typeGuard := c.detectTypeGuard(node.Condition)

		// 3. Check Consequence expression (potentially with narrowed environment)
		originalEnv := c.env
		narrowedEnv := c.applyTypeNarrowing(typeGuard)

		if narrowedEnv != nil {
			debugPrintf("// [Checker TernaryExpr] Applying type narrowing in consequence expression\n")
			c.env = narrowedEnv // Use narrowed environment for consequence
		}

		c.visit(node.Consequence)
		consType := node.Consequence.GetComputedType()

		// Restore original environment before checking alternative
		c.env = originalEnv

		// 4. Check Alternative expression - potentially with inverted type narrowing
		invertedEnv := c.applyInvertedTypeNarrowing(typeGuard)

		if invertedEnv != nil {
			debugPrintf("// [Checker TernaryExpr] Applying inverted type narrowing in alternative expression\n")
			c.env = invertedEnv // Use inverted narrowed environment for alternative
		}

		c.visit(node.Alternative)
		altType := node.Alternative.GetComputedType()

		// Restore original environment after alternative
		c.env = originalEnv

		// 5. Handle nil types from potential errors during visit
		if consType == nil {
			consType = types.Any
		}
		if altType == nil {
			altType = types.Any
		}

		var resultType types.Type
		// Basic type inference: if types match, use that type, otherwise Any.
		// TODO: Use Union types here when available.
		if consType == altType { // Pointer comparison works for primitives and Any
			resultType = consType
		} else {
			// Check structural equality for ArrayTypes (basic version)
			consArray, consIsArray := consType.(*types.ArrayType)
			altArray, altIsArray := altType.(*types.ArrayType)
			if consIsArray && altIsArray && consArray.ElementType == altArray.ElementType {
				resultType = consType // Types are equivalent array types
			} else {
				// TODO: Add structural checks for ObjectType, FunctionType?
				resultType = types.Any // Types differ, fallback to Any
			}
		}

		node.SetComputedType(resultType)

	case *parser.FunctionLiteral:
		c.checkFunctionLiteral(node)

	case *parser.ArrowFunctionLiteral:
		c.checkArrowFunctionLiteral(node)

	case *parser.MethodDefinition:
		c.checkMethodDefinition(node)

	case *parser.CallExpression:
		c.checkCallExpression(node)

	case *parser.YieldExpression:
		c.checkYieldExpression(node)

	case *parser.AwaitExpression:
		c.checkAwaitExpression(node)

	case *parser.AssignmentExpression:
		c.checkAssignmentExpression(node)

	case *parser.ArrayDestructuringAssignment:
		c.checkArrayDestructuringAssignment(node)

	case *parser.ObjectDestructuringAssignment:
		c.checkObjectDestructuringAssignment(node)

	case *parser.UpdateExpression:
		// --- NEW: Handle UpdateExpression ---
		c.visit(node.Argument)
		argType := node.Argument.GetComputedType()
		resultType := types.Number // Default result is number

		if argType == nil {
			argType = types.Any // Handle nil from visit error
		}

		widenedArgType := types.GetWidenedType(argType)

		// JavaScript allows ++/-- on any type that can be coerced to number
		// This includes: number, boolean, null, undefined, string
		// BigInt is special: ++/-- on bigint stays bigint (no conversion to number)
		// Objects/Arrays will coerce via ToPrimitive at runtime
		if widenedArgType == types.BigInt {
			// BigInt increment/decrement stays as BigInt
			resultType = types.BigInt
		}
		// For all other types (including boolean, string, null, undefined, any, objects),
		// allow them - they will be coerced to number at runtime

		// TODO: Check if the argument is assignable (e.g., not a literal 5++)
		// This might belong in a later compilation/resolution stage or require LHS checks here.

		node.SetComputedType(resultType)

	// --- NEW: Array/Index Type Checking ---
	case *parser.ArrayLiteral:
		c.checkArrayLiteral(node)

	case *parser.ObjectLiteral: // <<< ADD THIS CASE
		c.checkObjectLiteral(node)

	case *parser.IndexExpression:
		c.checkIndexExpression(node)

	// --- NEW: Member Expression Type Checking ---
	case *parser.MemberExpression:
		c.checkMemberExpression(node)

	// --- NEW: Optional Chaining Expression Type Checking ---
	case *parser.OptionalChainingExpression:
		c.checkOptionalChainingExpression(node)

	case *parser.OptionalIndexExpression:
		c.checkOptionalIndexExpression(node)

	case *parser.OptionalCallExpression:
		c.checkOptionalCallExpression(node)

	// --- Loop Statements (Control flow, check condition/body) ---
	case *parser.WhileStatement:
		c.visit(node.Condition)
		c.visit(node.Body)

	case *parser.DoWhileStatement:
		c.visit(node.Body)
		c.visit(node.Condition)

	case *parser.ForStatement:
		c.checkForStatement(node)

	case *parser.ForOfStatement:
		c.checkForOfStatement(node)

	case *parser.ForInStatement:
		c.checkForInStatement(node)

	// --- With Statement ---
	case *parser.WithStatement:
		c.checkWithStatement(node)

	// --- Loop Control (No specific type checking needed?) ---
	case *parser.BreakStatement:
		break // Nothing to check type-wise
	case *parser.EmptyStatement:
		break // Nothing to check type-wise for empty statements
	case *parser.ContinueStatement:
		break // Nothing to check type-wise

	case *parser.SwitchStatement: // Added
		c.checkSwitchStatement(node)

	// --- Parameter (visited within function context) ---
	case *parser.Parameter:
		c.visit(node.Name)
		c.visit(node.TypeAnnotation)
		// TODO: Resolve TypeAnnotation and store in node.ComputedType
		// TODO: Define param name in function scope environment

	// --- NEW: Handle RestParameter ---
	case *parser.RestParameter:
		c.visit(node.Name)
		if node.TypeAnnotation != nil {
			c.visit(node.TypeAnnotation)
			// Rest parameter type should already be resolved and set during function checking
			// This case handles visits to isolated RestParameter nodes if they occur
		}
		// Type should already be set during function literal processing

	case *parser.NewExpression:
		c.checkNewExpression(node)

	case *parser.ShorthandMethod:
		// Handle shorthand methods like methodName() { ... }
		// Similar to FunctionLiteral but specifically for method context

		// 1. Resolve explicit annotations and handle default values FIRST (similar to resolveFunctionLiteralType)
		paramTypes := make([]types.Type, len(node.Parameters))
		optionalParams := make([]bool, len(node.Parameters))

		// Create a temporary environment that will progressively accumulate parameters
		// This allows later parameters to reference earlier ones in their default values
		tempEnv := NewEnclosedEnvironment(c.env) // Create child environment

		for i, param := range node.Parameters {
			var resolvedParamType types.Type
			if param.TypeAnnotation != nil {
				resolvedParamType = c.resolveTypeAnnotation(param.TypeAnnotation)
			}

			// NEW: If no type annotation but has default value, infer type from default value
			if resolvedParamType == nil && param.DefaultValue != nil {
				// Use the temporary environment that includes previously defined parameters
				originalEnv := c.env
				c.env = tempEnv             // Use progressive environment that includes earlier parameters
				c.visit(param.DefaultValue) // This will set the computed type
				c.env = originalEnv         // Restore original environment

				defaultValueType := param.DefaultValue.GetComputedType()
				if defaultValueType != nil {
					// Widen literal types for parameter inference (like let/const inference)
					resolvedParamType = types.GetWidenedType(defaultValueType)
					debugPrintf("// [Checker ShorthandMethod] Inferred parameter '%s' type from default value: %s -> %s\n",
						param.Name.Value, defaultValueType.String(), resolvedParamType.String())
				}
			}

			if resolvedParamType == nil {
				resolvedParamType = types.Any // Default to Any if no annotation or resolution failed
			}
			paramTypes[i] = resolvedParamType

			// Add this parameter to the temporary environment BEFORE checking its default value
			// This way, the next parameter's default value can reference this parameter
			tempEnv.Define(param.Name.Value, resolvedParamType, false) // false = not const

			// Validate default value if present (skip if we already visited it for inference)
			if param.DefaultValue != nil && param.TypeAnnotation != nil {
				// Only validate if we had an explicit annotation (inference case already visited above)
				// Use the temporary environment that includes previously defined parameters
				originalEnv := c.env
				c.env = tempEnv             // Use progressive environment that includes earlier parameters
				c.visit(param.DefaultValue) // This will set the computed type
				c.env = originalEnv         // Restore original environment

				defaultValueType := param.DefaultValue.GetComputedType()
				if defaultValueType != nil && !types.IsAssignable(defaultValueType, resolvedParamType) {
					c.addError(param.DefaultValue, fmt.Sprintf("default value type '%s' is not assignable to parameter type '%s'", defaultValueType.String(), resolvedParamType.String()))
				}
			}

			// Parameter is optional if explicitly marked OR has a default value
			isOptional := param.Optional || (param.DefaultValue != nil)
			optionalParams[i] = isOptional
		}

		// Resolve return type annotation if present
		var resolvedReturnType types.Type
		if node.ReturnTypeAnnotation != nil {
			resolvedReturnType = c.resolveTypeAnnotation(node.ReturnTypeAnnotation)
		}

		// 2. Save outer return context
		outerExpectedReturnType := c.currentExpectedReturnType
		outerInferredReturnTypes := c.currentInferredReturnTypes

		// 3. Set context for body check
		c.currentExpectedReturnType = resolvedReturnType
		c.currentInferredReturnTypes = nil
		if resolvedReturnType == nil {
			c.currentInferredReturnTypes = []types.Type{}
		}

		// 4. Create function's inner scope & define parameters
		methodNameForLog := "<anonymous-method>"
		if node.Name != nil {
			methodNameForLog = node.Name.Value
		}
		debugPrintf("// [Checker Visit ShorthandMethod] Creating scope for method '%s'. Current Env: %p\n", methodNameForLog, c.env)
		originalEnv := c.env
		funcEnv := NewFunctionEnvironment(originalEnv)
		c.env = funcEnv

		// Define parameters in the method scope using resolved types
		for i, paramNode := range node.Parameters {
			if !funcEnv.Define(paramNode.Name.Value, paramTypes[i], false) {
				c.addError(paramNode.Name, fmt.Sprintf("duplicate parameter name: %s", paramNode.Name.Value))
			}
			paramNode.ComputedType = paramTypes[i]
		}

		// --- NEW: Handle rest parameter if present ---
		var restParameterType types.Type
		if node.RestParameter != nil {
			var resolvedRestType types.Type
			if node.RestParameter.TypeAnnotation != nil {
				resolvedRestType = c.resolveTypeAnnotation(node.RestParameter.TypeAnnotation)
				// Rest parameter type should be an array type
				if resolvedRestType != nil {
					if _, isArrayType := resolvedRestType.(*types.ArrayType); !isArrayType {
						c.addError(node.RestParameter.TypeAnnotation, fmt.Sprintf("rest parameter type must be an array type, got '%s'", resolvedRestType.String()))
						resolvedRestType = &types.ArrayType{ElementType: types.Any}
					}
				}
			}

			if resolvedRestType == nil {
				// Default to any[] if no annotation
				resolvedRestType = &types.ArrayType{ElementType: types.Any}
			}

			restParameterType = resolvedRestType

			// Define rest parameter in method scope
			if !funcEnv.Define(node.RestParameter.Name.Value, restParameterType, false) {
				c.addError(node.RestParameter.Name, fmt.Sprintf("duplicate parameter name: %s", node.RestParameter.Name.Value))
			}
			node.RestParameter.ComputedType = restParameterType
			debugPrintf("// [Checker ShorthandMethod] Defined rest parameter '%s' with type: %s\n", node.RestParameter.Name.Value, restParameterType.String())
		}
		// --- END NEW ---

		// 5. Visit method body
		c.visit(node.Body)

		// 6. Determine final return type
		var actualReturnType types.Type
		if resolvedReturnType != nil {
			actualReturnType = resolvedReturnType
		} else {
			if len(c.currentInferredReturnTypes) == 0 {
				actualReturnType = types.Void // Functions with no return statements have void return type
			} else {
				actualReturnType = types.NewUnionType(c.currentInferredReturnTypes...)
			}
			debugPrintf("// [Checker ShorthandMethod] Inferred return type for '%s': %s\n", methodNameForLog, actualReturnType.String())
		}

		// 7. Create the final function type with optional parameters using unified ObjectType
		methodSignature := &types.Signature{
			ParameterTypes:    paramTypes,
			ReturnType:        actualReturnType,
			OptionalParams:    optionalParams,
			IsVariadic:        node.RestParameter != nil, // Add variadic info
			RestParameterType: restParameterType,         // Add rest parameter type
		}
		finalMethodType := types.NewFunctionType(methodSignature)

		// 8. Set the computed type on the ShorthandMethod node
		debugPrintf("// [Checker ShorthandMethod] Setting computed type for '%s': %s\n", methodNameForLog, finalMethodType.String())
		node.SetComputedType(finalMethodType)

		// 9. Restore outer environment and context
		debugPrintf("// [Checker ShorthandMethod] Exiting '%s'. Restoring Env: %p\n", methodNameForLog, originalEnv)
		c.env = originalEnv
		c.currentExpectedReturnType = outerExpectedReturnType
		c.currentInferredReturnTypes = outerInferredReturnTypes

	// --- NEW: Handle SpreadElement ---
	case *parser.SpreadElement:
		// Visit the argument of the spread element
		c.visit(node.Argument)
		argType := node.Argument.GetComputedType()

		if argType == nil {
			argType = types.Any
		}

		// Allow spread elements to be type checked by their containing context
		// (e.g., ObjectLiteral, ArrayLiteral, CallExpression will do their own validation)
		// Only do basic type checking here
		node.SetComputedType(argType)

	// --- Class Declarations ---
	case *parser.ClassDeclaration:
		c.checkClassDeclaration(node)

	case *parser.ClassExpression:
		// Convert ClassExpression to ClassDeclaration for checking
		// The type checker treats them the same way
		var className *parser.Identifier
		if node.Name == nil {
			// Generate a unique name for anonymous classes
			// This follows the same pattern as the compiler
			anonymousName := fmt.Sprintf("__AnonymousClass_%d", c.getNextAnonymousId())
			className = &parser.Identifier{
				Token: node.Token,
				Value: anonymousName,
			}
		} else {
			className = node.Name
		}

		classDecl := &parser.ClassDeclaration{
			Token:          node.Token,
			Name:           className,
			TypeParameters: node.TypeParameters,
			SuperClass:     node.SuperClass,
			Body:           node.Body,
			IsAbstract:     node.IsAbstract,
		}
		c.checkClassDeclaration(classDecl)
		// Set the computed type on the expression node as well
		// Class expressions evaluate to their constructor function
		if ctorType, _, exists := c.env.Resolve(className.Value); exists {
			node.SetComputedType(ctorType)
		} else {
			node.SetComputedType(types.Any)
		}

	// --- Enum Declarations ---
	case *parser.EnumDeclaration:
		c.checkEnumDeclaration(node)

	// --- Labeled Statements ---
	case *parser.LabeledStatement:
		// Type check the labeled statement - labels themselves don't have types
		c.visit(node.Statement)

	// --- Exception Handling Statements ---
	case *parser.TryStatement:
		c.checkTryStatement(node)

	case *parser.ThrowStatement:
		c.checkThrowStatement(node)

	// --- Module System: Import/Export Statements ---
	case *parser.ImportDeclaration:
		c.checkImportDeclaration(node)

	case *parser.ExportNamedDeclaration:
		c.checkExportNamedDeclaration(node)

	case *parser.ExportDefaultDeclaration:
		c.checkExportDefaultDeclaration(node)

	case *parser.ExportAllDeclaration:
		c.checkExportAllDeclaration(node)

	// --- Type Expressions (normally handled via resolveTypeAnnotation, but may be visited directly) ---
	case *parser.ArrayTypeExpression:
		// Type expressions are normally resolved via resolveTypeAnnotation
		// If visited directly, we can set their computed type
		elemType := c.resolveTypeAnnotation(node.ElementType)
		if elemType == nil {
			elemType = types.Any
		}
		arrayType := &types.ArrayType{ElementType: elemType}
		node.SetComputedType(arrayType)

	case *parser.ComputedPropertyName:
		// Check the computed expression and set its type
		c.visit(node.Expr)
		// The type of a ComputedPropertyName is the type of its expression
		// This will be used as a key, so it should be string-like
		node.SetComputedType(node.Expr.GetComputedType())

	case *parser.ArrayParameterPattern:
		// Array destructuring pattern in catch clause or function parameters
		// Define all bindings with type 'any' (catch parameters are implicitly any)
		c.checkArrayParameterPattern(node)

	case *parser.ObjectParameterPattern:
		// Object destructuring pattern in catch clause or function parameters
		// Define all bindings with type 'any' (catch parameters are implicitly any)
		c.checkObjectParameterPattern(node)

	default:
		// Optional: Add error for unhandled node types
		c.addError(nil, fmt.Sprintf("Checker: Unhandled AST node type %T", node))
		break
	}
}

// checkArrayDestructuringDeclaration handles type checking for array destructuring declarations
func (c *Checker) checkArrayDestructuringDeclaration(node *parser.ArrayDestructuringDeclaration) {
	// Check if we have an initializer (required for const, optional for let/var)
	if node.Value == nil {
		if node.IsConst {
			c.addError(node, "const declaration must be initialized")
		}
		// For let/var without initializer, all variables get undefined type
		for _, element := range node.Elements {
			if element != nil && element.Target != nil {
				if ident, ok := element.Target.(*parser.Identifier); ok {
					if !c.env.Define(ident.Value, types.Undefined, node.IsConst) {
						c.addError(ident, fmt.Sprintf("identifier '%s' already declared", ident.Value))
					}
					ident.SetComputedType(types.Undefined)
				}
			}
		}
		return
	}

	// Check the RHS value
	c.visit(node.Value)
	valueType := node.Value.GetComputedType()
	if valueType == nil {
		valueType = types.Any
	}

	// Check if we have a type annotation
	var expectedType types.Type
	if node.TypeAnnotation != nil {
		expectedType = c.resolveTypeAnnotation(node.TypeAnnotation)
		// Verify that the value is assignable to the expected type
		if !types.IsAssignable(valueType, expectedType) {
			c.addError(node.Value, fmt.Sprintf("cannot assign type '%s' to type '%s'", valueType.String(), expectedType.String()))
		}
	}

	// Extract element types based on value type
	var elementTypes []types.Type
	if arrayType, ok := valueType.(*types.ArrayType); ok {
		// For arrays, all elements have the same type
		for range node.Elements {
			elementTypes = append(elementTypes, arrayType.ElementType)
		}
	} else if tupleType, ok := valueType.(*types.TupleType); ok {
		// For tuples, use specific element types
		elementTypes = tupleType.ElementTypes
	} else if valueType == types.Any {
		// For any type, all elements are any
		for range node.Elements {
			elementTypes = append(elementTypes, types.Any)
		}
	} else if _, ok := valueType.(*types.ObjectType); ok {
		// Object types might implement Symbol.iterator (iterable protocol)
		// At runtime, we'll check and use iterator protocol
		// For type checking, assume elements are Any since we can't statically determine iterator element type
		for range node.Elements {
			elementTypes = append(elementTypes, types.Any)
		}
	} else {
		// Not an array-like or iterable type
		c.addError(node.Value, fmt.Sprintf("cannot destructure non-array type '%s'", valueType.String()))
		// Continue with Any types to avoid cascading errors
		for range node.Elements {
			elementTypes = append(elementTypes, types.Any)
		}
	}

	// Process each destructuring element
	for i, element := range node.Elements {
		if element == nil || element.Target == nil {
			continue
		}

		// Get the element type (undefined if beyond array bounds)
		var elemType types.Type
		if element.IsRest {
			// Rest element gets an array of remaining elements
			if arrayType, ok := valueType.(*types.ArrayType); ok {
				// For arrays, rest gets the same element type
				elemType = &types.ArrayType{ElementType: arrayType.ElementType}
			} else if tupleType, ok := valueType.(*types.TupleType); ok {
				// For tuples, rest gets array of remaining element types
				if i < len(tupleType.ElementTypes) {
					remainingTypes := tupleType.ElementTypes[i:]
					if len(remainingTypes) == 0 {
						elemType = &types.ArrayType{ElementType: types.Never}
					} else if len(remainingTypes) == 1 {
						elemType = &types.ArrayType{ElementType: remainingTypes[0]}
					} else {
						unionType := &types.UnionType{Types: remainingTypes}
						elemType = &types.ArrayType{ElementType: unionType}
					}
				} else {
					elemType = &types.ArrayType{ElementType: types.Never}
				}
			} else {
				// For other types (like Any), rest gets any[]
				elemType = &types.ArrayType{ElementType: types.Any}
			}
		} else if i < len(elementTypes) {
			elemType = elementTypes[i]
		} else {
			elemType = types.Undefined
		}

		// Handle default value if present
		if element.Default != nil {
			c.visit(element.Default)
			defaultType := element.Default.GetComputedType()
			if defaultType == nil {
				defaultType = types.Any
			}

			// The resulting type is the union of element type and default type
			// Use the default type if element type is undefined or unknown
			if elemType == types.Undefined || elemType == types.Unknown {
				elemType = types.GetWidenedType(defaultType)
			}
		}

		// Define the variable(s) with inferred type - support both identifiers and nested patterns
		c.checkDestructuringTargetForDeclaration(element.Target, elemType, node.IsConst)
	}
}

// checkObjectDestructuringDeclaration handles type checking for object destructuring declarations
func (c *Checker) checkObjectDestructuringDeclaration(node *parser.ObjectDestructuringDeclaration) {
	// Add nil check for the node itself
	if node == nil {
		return
	}

	// Check if we have an initializer (required for const, optional for let/var)
	if node.Value == nil {
		if node.IsConst {
			c.addError(node, "const declaration must be initialized")
		}
		// For let/var without initializer, all variables get undefined type
		for _, prop := range node.Properties {
			if prop != nil && prop.Target != nil {
				if ident, ok := prop.Target.(*parser.Identifier); ok {
					if !c.env.Define(ident.Value, types.Undefined, node.IsConst) {
						c.addError(ident, fmt.Sprintf("identifier '%s' already declared", ident.Value))
					}
					ident.SetComputedType(types.Undefined)
				}
			}
		}

		// Handle rest property without initializer
		if node.RestProperty != nil {
			if ident, ok := node.RestProperty.Target.(*parser.Identifier); ok {
				if !c.env.Define(ident.Value, types.Undefined, node.IsConst) {
					c.addError(ident, fmt.Sprintf("identifier '%s' already declared", ident.Value))
				}
				ident.SetComputedType(types.Undefined)
			}
		}

		return
	}

	// Check the RHS value
	var valueType types.Type = types.Any
	if node.Value != nil {
		c.visit(node.Value)
		valueType = node.Value.GetComputedType()
		if valueType == nil {
			valueType = types.Any
		}
	}

	// Check if we have a type annotation
	var expectedType types.Type
	if node.TypeAnnotation != nil {
		expectedType = c.resolveTypeAnnotation(node.TypeAnnotation)
		// Verify that the value is assignable to the expected type
		if node.Value != nil && expectedType != nil && valueType != nil && !types.IsAssignable(valueType, expectedType) {
			c.addError(node.Value, fmt.Sprintf("cannot assign type '%s' to type '%s'", valueType.String(), expectedType.String()))
		}
	}

	// Check if the value is an object-like type (arrays are also objects in JavaScript)
	var objType *types.ObjectType
	var isArray bool
	if ot, ok := valueType.(*types.ObjectType); ok {
		objType = ot
	} else if _, ok := valueType.(*types.ArrayType); ok {
		// Arrays can be destructured as objects (e.g., {0: x, 1: y} from [10, 20])
		isArray = true
	} else if valueType != types.Any {
		// Not an object-like type
		c.addError(node.Value, fmt.Sprintf("cannot destructure non-object type '%s'", valueType.String()))
	}

	// Process each destructuring property
	for _, prop := range node.Properties {
		if prop == nil || prop.Key == nil || prop.Target == nil {
			continue
		}

		// Get the property type from the object
		var propType types.Type = types.Undefined
		var propName string
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			propName = keyIdent.Value
			if objType != nil {
				if pt, exists := objType.Properties[propName]; exists {
					propType = pt
				}
			} else if isArray {
				// For arrays, numeric keys access array elements
				if arrType, ok := valueType.(*types.ArrayType); ok {
					propType = arrType.ElementType
				}
			} else if valueType == types.Any {
				propType = types.Any
			}
		} else {
			// Computed property - can't check statically
			propType = types.Any
		}

		// Handle default value if present
		if prop.Default != nil {
			c.visit(prop.Default)
			defaultType := prop.Default.GetComputedType()
			if defaultType == nil {
				defaultType = types.Any
			}

			// The resulting type is the union of property type and default type
			// Use the default type if property type is undefined or unknown
			if propType == types.Undefined || propType == types.Unknown {
				propType = types.GetWidenedType(defaultType)
			}
		}

		// Define the variable(s) with inferred type - support both identifiers and nested patterns
		c.checkDestructuringTargetForDeclaration(prop.Target, propType, node.IsConst)
	}

	// Handle rest property if present
	if node.RestProperty != nil {
		// Rest property gets an object type containing all remaining properties
		var restType types.Type

		if valueType == types.Any {
			// If RHS is Any, rest property is also Any
			restType = types.Any
		} else if objType != nil {
			// Create a new object type excluding the destructured properties
			extractedProps := make(map[string]struct{})
			for _, prop := range node.Properties {
				if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
					extractedProps[keyIdent.Value] = struct{}{}
				}
				// Skip computed properties (can't determine statically)
			}

			// Build remaining properties map
			remainingProps := make(map[string]types.Type)
			for propName, propType := range objType.Properties {
				if _, wasExtracted := extractedProps[propName]; !wasExtracted {
					remainingProps[propName] = propType
				}
			}

			// Create object type with remaining properties
			restType = &types.ObjectType{Properties: remainingProps}
		} else {
			// For other types, rest gets an empty object type
			restType = &types.ObjectType{Properties: make(map[string]types.Type)}
		}

		// Define the rest variable
		if ident, ok := node.RestProperty.Target.(*parser.Identifier); ok {
			if !c.env.Define(ident.Value, restType, node.IsConst) {
				c.addError(ident, fmt.Sprintf("identifier '%s' already declared", ident.Value))
			}
			ident.SetComputedType(restType)
		}
	}
}

// visitWithContext is the context-aware dispatch method for AST traversal.
// It passes expected type information to expressions that can benefit from contextual typing.
func (c *Checker) visitWithContext(node parser.Node, context *ContextualType) {
	if node == nil {
		return
	}

	// If no context provided, fall back to regular visit
	if context == nil || context.ExpectedType == nil {
		c.visit(node)
		return
	}

	debugPrintf("// [Checker VisitContext] Node: %T, Expected: %s\n", node, context.ExpectedType.String())

	// Handle specific node types that benefit from contextual typing
	switch node := node.(type) {
	case *parser.ArrayLiteral:
		c.checkArrayLiteralWithContext(node, context)
	case *parser.ObjectLiteral:
		c.checkObjectLiteralWithContext(node, context)
	case *parser.ArrowFunctionLiteral:
		c.checkArrowFunctionLiteralWithContext(node, context)
	default:
		// For other node types, use regular visit for now
		c.visit(node)
	}
}

// checkArrowFunctionLiteralWithContext handles contextual typing for arrow functions
func (c *Checker) checkArrowFunctionLiteralWithContext(node *parser.ArrowFunctionLiteral, context *ContextualType) {
	// Check if the expected type is a function type
	if objType, ok := context.ExpectedType.(*types.ObjectType); ok && objType.IsCallable() && len(objType.CallSignatures) > 0 {
		expectedSig := objType.CallSignatures[0] // Use the first signature for contextual typing

		// Check if parameter counts are compatible
		nodeParamCount := len(node.Parameters)
		expectedParamCount := len(expectedSig.ParameterTypes)

		// Only apply contextual typing if the parameter counts match and none of the arrow function
		// parameters have explicit type annotations
		canApplyContextualTyping := (nodeParamCount == expectedParamCount)
		for _, param := range node.Parameters {
			if param.TypeAnnotation != nil {
				canApplyContextualTyping = false
				break
			}
		}

		if canApplyContextualTyping {
			debugPrintf("// [Checker ArrowFuncContext] Applying contextual typing from signature: %s\n", expectedSig.String())

			// Check if the return type is generic - if so, try to use its constraint for contextual typing
			var contextualReturnType types.Type = nil
			if expectedSig.ReturnType != nil {
				// If it's a type parameter, try to use its constraint for contextual typing
				if typeParam, isTypeParam := expectedSig.ReturnType.(*types.TypeParameterType); isTypeParam {
					if typeParam.Parameter != nil && typeParam.Parameter.Constraint != nil {
						contextualReturnType = typeParam.Parameter.Constraint
						debugPrintf("// [Checker ArrowFuncContext] Using type parameter constraint as contextual return type: %s -> %s\n",
							expectedSig.ReturnType.String(), contextualReturnType.String())
					} else {
						debugPrintf("// [Checker ArrowFuncContext] Skipping type parameter without constraint for inference: %s\n", expectedSig.ReturnType.String())
					}
				} else {
					// Use concrete return type directly
					contextualReturnType = expectedSig.ReturnType
					debugPrintf("// [Checker ArrowFuncContext] Using concrete contextual return type: %s\n", contextualReturnType.String())
				}
			}

			// Create a modified arrow function context with inferred parameter types
			ctx := &FunctionCheckContext{
				FunctionName:             "<arrow>",
				TypeParameters:           node.TypeParameters,
				Parameters:               node.Parameters,
				RestParameter:            node.RestParameter,
				ReturnTypeAnnotation:     node.ReturnTypeAnnotation,
				Body:                     node.Body,
				IsArrow:                  true,
				IsGenerator:              false, // Arrow functions cannot be generators
				AllowSelfReference:       false,
				AllowOverloadCompletion:  false,
				ContextualParameterTypes: expectedSig.ParameterTypes, // Pass contextual parameter types
				ContextualReturnType:     contextualReturnType,       // Only pass non-generic return types
			}

			// 1. Resolve parameters with contextual types
			preliminarySignature, paramTypes, paramNames, restParameterType, restParameterName, typeParamEnv := c.resolveFunctionParametersWithContext(ctx)

			// 2. Setup function environment
			originalEnv := c.setupFunctionEnvironment(ctx, paramTypes, paramNames, restParameterType, restParameterName, preliminarySignature, typeParamEnv)

			// 3. Check function body and determine return type
			// If we want return type inference (contextualReturnType is nil), pass nil to checkFunctionBody
			var expectedReturnTypeForBodyCheck types.Type
			if ctx.ContextualReturnType != nil {
				expectedReturnTypeForBodyCheck = preliminarySignature.ReturnType
				if expectedReturnTypeForBodyCheck != nil {
					debugPrintf("// [Checker ArrowFuncContext] Using contextual return type for body check: %s\n", expectedReturnTypeForBodyCheck.String())
				} else {
					debugPrintf("// [Checker ArrowFuncContext] Using contextual return type for body check: <nil>\n")
				}
			} else {
				expectedReturnTypeForBodyCheck = nil // Signal that we want inference
				debugPrintf("// [Checker ArrowFuncContext] Using return type inference (nil expected return type)\n")
			}
			finalReturnType := c.checkFunctionBody(ctx, expectedReturnTypeForBodyCheck)
			debugPrintf("// [Checker ArrowFuncContext] Inferred final return type: %s\n", finalReturnType.String())

			// Check if we used a type parameter constraint for contextual typing
			// If so, the final return type should be the original type parameter, not the constraint
			if expectedSig.ReturnType != nil {
				if typeParam, isTypeParam := expectedSig.ReturnType.(*types.TypeParameterType); isTypeParam {
					if typeParam.Parameter != nil && typeParam.Parameter.Constraint != nil && contextualReturnType != nil {
						// We used the constraint for inference, but the final type should be the type parameter
						finalReturnType = expectedSig.ReturnType
						debugPrintf("// [Checker ArrowFuncContext] Overriding final return type to type parameter: %s\n", finalReturnType.String())
					}
				}
			}

			// 4. Create final function type
			finalFuncType := c.createFinalFunctionType(ctx, paramTypes, finalReturnType, restParameterType)

			// 5. Set computed type on the ArrowFunctionLiteral node
			debugPrintf("// [Checker ArrowFuncContext] Setting contextual computed type: %s\n", finalFuncType.String())
			node.SetComputedType(finalFuncType)

			// 6. Restore environment
			c.env = originalEnv
			return
		}
	}

	// Fallback to regular arrow function checking if contextual typing can't be applied
	debugPrintf("// [Checker ArrowFuncContext] Cannot apply contextual typing, falling back to regular check\n")
	c.checkArrowFunctionLiteral(node)
}

// --- Exception Handling Type Checking ---

// checkTryStatement performs type checking for try/catch statements
func (c *Checker) checkTryStatement(node *parser.TryStatement) {
	// Check the try block
	c.visit(node.Body)

	// Check the catch clause if present
	if node.CatchClause != nil {
		c.checkCatchClause(node.CatchClause)
	}
}

// checkCatchClause performs type checking for catch clauses
func (c *Checker) checkCatchClause(clause *parser.CatchClause) {
	// Create a new environment for the catch block
	originalEnv := c.env
	c.env = NewEnclosedEnvironment(c.env)

	// Define the catch parameter if present
	if clause.Parameter != nil {
		// In JavaScript/TypeScript, catch parameter is implicitly 'any' type
		switch param := clause.Parameter.(type) {
		case *parser.Identifier:
			// Simple identifier: catch (e)
			if !c.env.Define(param.Value, types.Any, false) {
				c.addError(param, fmt.Sprintf("parameter '%s' already declared", param.Value))
			}
			param.SetComputedType(types.Any)
		case *parser.ArrayParameterPattern, *parser.ObjectParameterPattern:
			// Destructuring pattern: catch ([x, y]) or catch ({message})
			// Type check the pattern and define all bindings
			c.visit(clause.Parameter)
			// The visit will define all bindings from the pattern
		default:
			c.addError(clause.Parameter, fmt.Sprintf("unexpected catch parameter type: %T", param))
		}
	}

	// Check the catch body
	c.visit(clause.Body)

	// Restore the original environment
	c.env = originalEnv
}

// checkArrayParameterPattern handles type checking for array destructuring patterns in catch clauses
func (c *Checker) checkArrayParameterPattern(node *parser.ArrayParameterPattern) {
	// In catch clauses, all bindings are type 'any'
	for _, element := range node.Elements {
		if element == nil || element.Target == nil {
			continue
		}
		c.definePatternBinding(element.Target, types.Any)
	}
	node.SetComputedType(types.Any)
}

// checkObjectParameterPattern handles type checking for object destructuring patterns in catch clauses
func (c *Checker) checkObjectParameterPattern(node *parser.ObjectParameterPattern) {
	// In catch clauses, all bindings are type 'any'
	for _, prop := range node.Properties {
		if prop == nil || prop.Target == nil {
			continue
		}
		c.definePatternBinding(prop.Target, types.Any)
	}
	// Handle rest property if present
	if node.RestProperty != nil && node.RestProperty.Target != nil {
		c.definePatternBinding(node.RestProperty.Target, types.Any)
	}
	node.SetComputedType(types.Any)
}

// definePatternBinding defines a binding from a destructuring pattern (helper for catch clauses)
func (c *Checker) definePatternBinding(target parser.Expression, bindingType types.Type) {
	switch t := target.(type) {
	case *parser.Identifier:
		if !c.env.Define(t.Value, bindingType, false) {
			c.addError(t, fmt.Sprintf("parameter '%s' already declared", t.Value))
		}
		t.SetComputedType(bindingType)
	case *parser.ArrayParameterPattern:
		// Nested array parameter pattern
		for _, element := range t.Elements {
			if element != nil && element.Target != nil {
				c.definePatternBinding(element.Target, bindingType)
			}
		}
		t.SetComputedType(bindingType)
	case *parser.ArrayLiteral:
		// Nested array literal pattern (e.g., catch ([[x, y], z]))
		for _, element := range t.Elements {
			if element != nil {
				c.definePatternBinding(element, bindingType)
			}
		}
		t.SetComputedType(bindingType)
	case *parser.ObjectParameterPattern:
		// Nested object parameter pattern
		for _, prop := range t.Properties {
			if prop != nil && prop.Target != nil {
				c.definePatternBinding(prop.Target, bindingType)
			}
		}
		if t.RestProperty != nil && t.RestProperty.Target != nil {
			c.definePatternBinding(t.RestProperty.Target, bindingType)
		}
		t.SetComputedType(bindingType)
	case *parser.ObjectLiteral:
		// Nested object literal pattern (e.g., catch ({a: {b, c}}))
		for _, prop := range t.Properties {
			if prop != nil && prop.Value != nil {
				c.definePatternBinding(prop.Value, bindingType)
			}
		}
		t.SetComputedType(bindingType)
	default:
		c.addError(target, fmt.Sprintf("unexpected pattern binding target: %T", t))
	}
}

// checkThrowStatement performs type checking for throw statements
func (c *Checker) checkThrowStatement(node *parser.ThrowStatement) {
	// Check that throw has an expression
	if node.Value == nil {
		c.addError(node, "throw statement requires an expression")
		return
	}

	// Visit the expression being thrown
	c.visit(node.Value)

	// In TypeScript, throw expressions have type 'never'
	// but for simplicity in Phase 1, we don't need to enforce much
	// The expression can be of any type (JavaScript allows throwing anything)
}

// blockContainsThrow checks if a block statement contains a throw statement
func (c *Checker) blockContainsThrow(block *parser.BlockStatement) bool {
	if block == nil {
		return false
	}

	for _, stmt := range block.Statements {
		if c.statementContainsThrow(stmt) {
			return true
		}
	}
	return false
}

// statementContainsThrow checks if a statement contains a throw (including nested)
func (c *Checker) statementContainsThrow(stmt parser.Statement) bool {
	switch node := stmt.(type) {
	case *parser.ThrowStatement:
		return true
	case *parser.BlockStatement:
		return c.blockContainsThrow(node)
	case *parser.IfStatement:
		// Check both branches
		if c.statementContainsThrow(node.Consequence) {
			return true
		}
		if node.Alternative != nil && c.statementContainsThrow(node.Alternative) {
			return true
		}
		return false
	case *parser.ExpressionStatement:
		// Could contain throw in nested expressions, but for now we'll keep it simple
		return false
	default:
		// For other statement types, assume no throw for simplicity
		return false
	}
}

// GetProgram returns the program AST being checked
func (c *Checker) GetProgram() *parser.Program {
	return c.program
}

// extractInferredTypeArguments extracts the inferred type arguments from a solved type inference
// It returns an ordered list of types that matches the order of type parameters in the generic type
func (c *Checker) extractInferredTypeArguments(genericType *types.GenericType, originalSig, inferredSig *types.Signature) []types.Type {
	// Create a map from type parameter to inferred type by comparing original and inferred signatures
	solution := make(map[*types.TypeParameter]types.Type)

	// Helper to extract type parameter mappings by comparing types
	var extractMappings func(original, inferred types.Type)
	extractMappings = func(original, inferred types.Type) {
		switch origType := original.(type) {
		case *types.TypeParameterType:
			// Found a type parameter - map it to the inferred type
			solution[origType.Parameter] = inferred
		case *types.ArrayType:
			if infArray, ok := inferred.(*types.ArrayType); ok {
				extractMappings(origType.ElementType, infArray.ElementType)
			}
		case *types.UnionType:
			if infUnion, ok := inferred.(*types.UnionType); ok && len(origType.Types) == len(infUnion.Types) {
				for i := range origType.Types {
					extractMappings(origType.Types[i], infUnion.Types[i])
				}
			}
			// Add more cases as needed
		}
	}

	// Compare parameter types
	for i := range originalSig.ParameterTypes {
		if i < len(inferredSig.ParameterTypes) {
			extractMappings(originalSig.ParameterTypes[i], inferredSig.ParameterTypes[i])
		}
	}

	// Build ordered type arguments based on generic type's type parameters
	var typeArgs []types.Type
	for _, typeParam := range genericType.TypeParameters {
		if inferredType, found := solution[typeParam]; found {
			typeArgs = append(typeArgs, inferredType)
			debugPrintf("// [Checker ExtractInferred] %s = %s\n", typeParam.Name, inferredType.String())
		} else {
			// Type parameter not inferred - use Any as fallback
			typeArgs = append(typeArgs, types.Any)
			debugPrintf("// [Checker ExtractInferred] %s = any (not inferred)\n", typeParam.Name)
		}
	}

	return typeArgs
}

// ----------------------------------------------------------------------------
// Module System: Environment Setup and Integration
// ----------------------------------------------------------------------------

// EnableModuleMode sets up the checker for module-aware type checking
func (c *Checker) EnableModuleMode(modulePath string, moduleLoader modules.ModuleLoader) {
	if c.env == nil {
		// Create a base environment
		c.env = NewEnvironment()
	}

	c.moduleEnv = NewModuleEnvironment(c.env, modulePath, moduleLoader)
	c.moduleLoader = moduleLoader
}

// IsModuleMode returns true if the checker is in module-aware mode
func (c *Checker) IsModuleMode() bool {
	return c.moduleEnv != nil
}

// GetImportBindings returns all import bindings from the module environment
// This is used by the compiler to synchronize import information
func (c *Checker) GetImportBindings() map[string]*ImportBinding {
	if !c.IsModuleMode() {
		return nil
	}
	return c.moduleEnv.ImportedNames
}

// GetModuleExports returns the exports from the current module (if in module mode)
func (c *Checker) GetModuleExports() map[string]types.Type {
	debugPrintf("// [Checker GetModuleExports] Called. moduleEnv=%v\n", c.moduleEnv != nil)
	if c.moduleEnv != nil {
		exports := c.moduleEnv.GetAllExports()
		debugPrintf("// [Checker GetModuleExports] Returning %d exports\n", len(exports))
		return exports
	}
	debugPrintf("// [Checker GetModuleExports] No moduleEnv, returning empty map\n")
	return make(map[string]types.Type)
}

// ----------------------------------------------------------------------------
// Module System: Import/Export Type Checking
// ----------------------------------------------------------------------------

// checkImportDeclaration handles type checking for import statements
func (c *Checker) checkImportDeclaration(node *parser.ImportDeclaration) {
	if node.Source == nil {
		c.addError(node, "import statement missing source module")
		return
	}

	sourceModulePath := node.Source.Value
	debugPrintf("// [Checker] Processing import from: %s (IsTypeOnly: %v)\n", sourceModulePath, node.IsTypeOnly)

	// Handle bare imports (side-effect only)
	if len(node.Specifiers) == 0 {
		debugPrintf("// [Checker] Bare import (side effects only): %s\n", sourceModulePath)
		// For bare imports, we just need to ensure the module can be resolved
		// No bindings are created in the local environment
		return
	}

	// Validate and process import specifiers
	for _, spec := range node.Specifiers {
		switch importSpec := spec.(type) {
		case *parser.ImportDefaultSpecifier:
			// Default import: import defaultName from "module"
			if importSpec.Local == nil {
				c.addError(node, "import default specifier missing local name")
				continue
			}

			localName := importSpec.Local.Value
			c.processImportBinding(localName, sourceModulePath, "default", ImportDefault, node.IsTypeOnly)

		case *parser.ImportNamedSpecifier:
			// Named import: import { name } or import { name as alias }
			if importSpec.Local == nil || importSpec.Imported == nil {
				c.addError(node, "import named specifier missing names")
				continue
			}

			localName := importSpec.Local.Value
			importedName := importSpec.Imported.Value
			// Use per-specifier type-only flag if available, otherwise fall back to declaration-level flag
			isTypeOnly := importSpec.IsTypeOnly || node.IsTypeOnly
			c.processImportBinding(localName, sourceModulePath, importedName, ImportNamed, isTypeOnly)

		case *parser.ImportNamespaceSpecifier:
			// Namespace import: import * as name from "module"
			if importSpec.Local == nil {
				c.addError(node, "import namespace specifier missing local name")
				continue
			}

			localName := importSpec.Local.Value
			c.processImportBinding(localName, sourceModulePath, "*", ImportNamespace, node.IsTypeOnly)

		default:
			c.addError(node, fmt.Sprintf("unknown import specifier type: %T", spec))
		}
	}
}

// processImportBinding handles the binding of an imported name
func (c *Checker) processImportBinding(localName, sourceModule, sourceName string, importType ImportBindingType, isTypeOnly bool) {
	// If we're in module mode, use the module environment for proper tracking
	if c.IsModuleMode() {
		c.moduleEnv.DefineImport(localName, sourceModule, sourceName, importType)

		// Try to resolve the actual type from the source module
		resolvedType := c.moduleEnv.ResolveImportedType(localName)
		if resolvedType != types.Any {
			debugPrintf("// [Checker] Imported %s: %s = %s (resolved, type-only: %v)\n", localName, sourceName, resolvedType.String(), isTypeOnly)

			// Always register the imported type in the local type environment for type annotation resolution
			// This allows interfaces and type aliases to be used in type annotations
			c.env.DefineTypeAlias(localName, resolvedType)
			debugPrintf("// [Checker] Registered imported type %s in local type environment\n", localName)

			// For type-only imports, don't register in the value environment
			if !isTypeOnly {
				// If this is a class (constructor function), also register it in the value environment
				// Classes need to be available for both type annotations and runtime usage (new expressions)
				if objectType, ok := resolvedType.(*types.ObjectType); ok {
					if len(objectType.ConstructSignatures) > 0 {
						// This is a constructor function type (class)
						c.env.Define(localName, resolvedType, false)
						debugPrintf("// [Checker] Registered imported class %s in value environment\n", localName)
					}
				}
			} else {
				debugPrintf("// [Checker] Skipped value environment registration for type-only import %s\n", localName)
			}
		} else {
			debugPrintf("// [Checker] Imported %s: %s = any (unresolved, type-only: %v)\n", localName, sourceName, isTypeOnly)
			// Even if unresolved, we should still register the name as a type alias if it's type-only
			// This allows type annotations to work even when the actual type can't be resolved at compile time
			if isTypeOnly {
				c.env.DefineTypeAlias(localName, types.Any)
				debugPrintf("// [Checker] Registered unresolved type-only import %s as type alias\n", localName)
			}
		}
	} else {
		// Fallback: bind to 'any' type in regular environment
		// For type-only imports, don't create runtime bindings
		if !isTypeOnly {
			c.env.Define(localName, types.Any, false)
			debugPrintf("// [Checker] Imported %s: %s = any (no module mode)\n", localName, sourceName)
		} else {
			debugPrintf("// [Checker] Skipped import binding for type-only import %s (no module mode)\n", localName)
		}
	}
}

// checkExportNamedDeclaration handles type checking for named export statements
func (c *Checker) checkExportNamedDeclaration(node *parser.ExportNamedDeclaration) {
	if node.Declaration != nil {
		// Direct export: export const x = 1; export function foo() {}
		debugPrintf("// [Checker] Processing direct export declaration\n")
		c.visit(node.Declaration)

		// Extract and register exported names
		c.processExportDeclaration(node.Declaration)

	} else if len(node.Specifiers) > 0 {
		// Named exports: export { x, y } or export { x } from "module"
		debugPrintf("// [Checker] Processing named export specifiers\n")

		if node.Source != nil {
			// Re-export: export { x } from "module"
			sourceModule := node.Source.Value
			debugPrintf("// [Checker] Re-export from: %s\n", sourceModule)

			for _, spec := range node.Specifiers {
				if exportSpec, ok := spec.(*parser.ExportNamedSpecifier); ok {
					localName := getExportSpecName(exportSpec.Local)
					exportName := getExportSpecName(exportSpec.Exported)

					if c.IsModuleMode() {
						c.moduleEnv.DefineReExport(exportName, sourceModule, localName, node.IsTypeOnly)
						debugPrintf("// [Checker] Re-exported: %s as %s from %s\n", localName, exportName, sourceModule)
					}
				}
			}
		} else {
			// Local export: export { x, y }
			// Validate that the exported names exist in current scope and register them
			for _, spec := range node.Specifiers {
				if exportSpec, ok := spec.(*parser.ExportNamedSpecifier); ok {
					if exportSpec.Local == nil {
						c.addError(node, "export specifier missing local name")
						continue
					}

					localName := getExportSpecName(exportSpec.Local)
					exportName := getExportSpecName(exportSpec.Exported)

					// Check if the local name exists in current scope
					if localType, _, exists := c.env.Resolve(localName); exists {
						// Register the export in module environment
						if c.IsModuleMode() {
							c.moduleEnv.DefineExport(localName, exportName, localType, nil)
						}
						debugPrintf("// [Checker] Exported: %s as %s (type: %s)\n", localName, exportName, localType.String())
					} else {
						c.addError(node, fmt.Sprintf("exported name '%s' not found in current scope", localName))
					}
				}
			}
		}
	}
}

// checkExportDefaultDeclaration handles type checking for default export statements
func (c *Checker) checkExportDefaultDeclaration(node *parser.ExportDefaultDeclaration) {
	if node.Declaration == nil {
		c.addError(node, "export default statement missing declaration")
		return
	}

	debugPrintf("// [Checker] Processing default export\n")

	// Type check the default export expression
	c.visit(node.Declaration)

	// Register the default export
	if c.IsModuleMode() {
		exportType := node.Declaration.GetComputedType()
		if exportType == nil {
			exportType = types.Any
		}

		c.moduleEnv.DefineExport("default", "default", exportType, nil)
		debugPrintf("// [Checker] Default export registered (type: %s)\n", exportType.String())
	} else {
		debugPrintf("// [Checker] Default export processed (no module mode)\n")
	}
}

// checkExportAllDeclaration handles type checking for export all statements
func (c *Checker) checkExportAllDeclaration(node *parser.ExportAllDeclaration) {
	if node.Source == nil {
		c.addError(node, "export * statement missing source module")
		return
	}

	sourceModule := node.Source.Value
	debugPrintf("// [Checker] Processing export%s * from: %s\n",
		func() string {
			if node.IsTypeOnly {
				return " type"
			} else {
				return ""
			}
		}(), sourceModule)

	if node.Exported != nil {
		// export * as name from "module"
		exportName := node.Exported.Value
		debugPrintf("// [Checker] Export all as namespace: %s\n", exportName)

		if c.IsModuleMode() {
			// This creates a namespace export that contains all exports from the source module
			c.moduleEnv.DefineReExport(exportName, sourceModule, "*", node.IsTypeOnly)
		}
	} else {
		// export * from "module" - re-exports all named exports (but not default)
		debugPrintf("// [Checker] Export all (anonymous) from: %s\n", sourceModule)

		if c.IsModuleMode() {
			// Mark dependency first
			c.moduleEnv.Dependencies[sourceModule] = true

			// Resolve and expand exports from the source module
			if c.moduleLoader != nil {
				moduleRecord, err := c.moduleLoader.LoadModule(sourceModule, c.source.Path)
				if err != nil {
					c.addError(node, "failed to load source module")
					return
				}

				// Get all export names from the source module (excluding default export)
				exportNames := moduleRecord.GetExportNames()
				debugPrintf("// [Checker] Source module '%s' has %d exports: %v\n", sourceModule, len(exportNames), exportNames)

				// Create individual re-export bindings for each named export
				for _, exportName := range exportNames {
					if exportName != "default" { // Skip default export in export *
						debugPrintf("// [Checker] Re-exporting '%s' from '%s'\n", exportName, sourceModule)
						c.moduleEnv.DefineReExport(exportName, sourceModule, exportName, node.IsTypeOnly)
					}
				}

				debugPrintf("// [Checker] Completed export * expansion for %d exports from '%s'\n", len(exportNames), sourceModule)
			} else {
				c.addError(node, "module loader not available for resolving exports")
			}
		}
	}
}

// processExportDeclaration processes a declaration that's being exported directly
func (c *Checker) processExportDeclaration(decl parser.Statement) {
	switch node := decl.(type) {
	case *parser.LetStatement:
		if node.Name != nil {
			localName := node.Name.Value
			// Look up the type from the environment (the statement was already processed)
			exportType, _, exists := c.env.Resolve(localName)
			if !exists {
				exportType = types.Any
			}

			if c.IsModuleMode() {
				c.moduleEnv.DefineExport(localName, localName, exportType, decl)
			}
			debugPrintf("// [Checker] Exported let: %s (type: %s)\n", localName, exportType.String())
		}

	case *parser.ConstStatement:
		if node.Name != nil {
			localName := node.Name.Value
			// Look up the type from the environment (the statement was already processed)
			exportType, _, exists := c.env.Resolve(localName)
			if !exists {
				exportType = types.Any
			}

			if c.IsModuleMode() {
				c.moduleEnv.DefineExport(localName, localName, exportType, decl)
			}
			debugPrintf("// [Checker] Exported const: %s (type: %s)\n", localName, exportType.String())
		}

	case *parser.VarStatement:
		if node.Name != nil {
			localName := node.Name.Value
			// Look up the type from the environment (the statement was already processed)
			exportType, _, exists := c.env.Resolve(localName)
			if !exists {
				exportType = types.Any
			}

			if c.IsModuleMode() {
				c.moduleEnv.DefineExport(localName, localName, exportType, decl)
			}
			debugPrintf("// [Checker] Exported var: %s (type: %s)\n", localName, exportType.String())
		}

	case *parser.ExpressionStatement:
		// Functions and classes are expressions wrapped in ExpressionStatement
		switch expr := node.Expression.(type) {
		case *parser.FunctionLiteral:
			if expr.Name != nil {
				localName := expr.Name.Value
				exportType := expr.GetComputedType()
				if exportType == nil {
					exportType = types.Any
				}

				if c.IsModuleMode() {
					c.moduleEnv.DefineExport(localName, localName, exportType, decl)
				}
				debugPrintf("// [Checker] Exported function: %s (type: %s)\n", localName, exportType.String())
			}

		case *parser.ClassExpression:
			if expr.Name != nil {
				localName := expr.Name.Value
				exportType := expr.GetComputedType()
				if exportType == nil {
					exportType = types.Any
				}

				if c.IsModuleMode() {
					c.moduleEnv.DefineExport(localName, localName, exportType, decl)
				}
				debugPrintf("// [Checker] Exported class: %s (type: %s)\n", localName, exportType.String())
			}

		case *parser.EnumDeclaration:
			if expr.Name != nil {
				localName := expr.Name.Value
				exportType := expr.GetComputedType()
				if exportType == nil {
					exportType = types.Any
				}

				if c.IsModuleMode() {
					c.moduleEnv.DefineExport(localName, localName, exportType, decl)
				}
				debugPrintf("// [Checker] Exported enum: %s (type: %s)\n", localName, exportType.String())
			}

		default:
			debugPrintf("// [Checker] Exported expression: %T\n", expr)
		}

	case *parser.InterfaceDeclaration:
		if node.Name != nil {
			localName := node.Name.Value
			// For interfaces, look up in the type environment
			exportType, exists := c.env.ResolveType(localName)
			if !exists {
				exportType = types.Any
			}

			if c.IsModuleMode() {
				c.moduleEnv.DefineExport(localName, localName, exportType, decl)
			}
			debugPrintf("// [Checker] Exported interface: %s (type: %s)\n", localName, exportType.String())
		}

	case *parser.TypeAliasStatement:
		if node.Name != nil {
			localName := node.Name.Value
			// For type aliases, look up in the type environment
			exportType, exists := c.env.ResolveType(localName)
			if !exists {
				exportType = types.Any
			}

			if c.IsModuleMode() {
				c.moduleEnv.DefineExport(localName, localName, exportType, decl)
			}
			debugPrintf("// [Checker] Exported type: %s (type: %s)\n", localName, exportType.String())
		}

	default:
		debugPrintf("// [Checker] Exported unknown declaration type: %T\n", decl)
	}
}

// getAssignmentErrorTypes returns appropriate type strings for assignment error messages
func (c *Checker) getAssignmentErrorTypes(sourceType, targetType types.Type) (string, string) {
	var sourceTypeStr, targetTypeStr string

	// For most cases, use literal types for source to match TypeScript behavior
	sourceTypeStr = sourceType.String()

	// For target type, handle enum types specially
	if enumType, ok := targetType.(*types.UnionType); ok {
		// Check if this is an enum union type
		if len(enumType.Types) > 0 {
			if types.IsEnumMemberType(enumType.Types[0]) {
				if memberType, ok := enumType.Types[0].(*types.EnumMemberType); ok {
					targetTypeStr = memberType.EnumName
				} else {
					targetTypeStr = targetType.String()
				}
			} else {
				targetTypeStr = targetType.String()
			}
		} else {
			targetTypeStr = targetType.String()
		}
	} else {
		targetTypeStr = targetType.String()
	}

	return sourceTypeStr, targetTypeStr
}

// getEnumAssignmentErrorTypes returns appropriate type strings for enum assignment error messages
func (c *Checker) getEnumAssignmentErrorTypes(sourceType, targetType types.Type) (string, string) {
	var sourceTypeStr, targetTypeStr string

	// For enum assignments, use widened source type for better error messages
	if literalType, ok := sourceType.(*types.LiteralType); ok {
		switch literalType.Value.Type() {
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			sourceTypeStr = "number"
		case vm.TypeString:
			sourceTypeStr = "string"
		case vm.TypeBoolean:
			sourceTypeStr = "boolean"
		default:
			sourceTypeStr = sourceType.String()
		}
	} else {
		sourceTypeStr = sourceType.String()
	}

	// For enum target types, use the enum name or full string representation
	if enumType, ok := targetType.(*types.UnionType); ok {
		// Check if this is an enum union type
		if len(enumType.Types) > 0 {
			if types.IsEnumMemberType(enumType.Types[0]) {
				if memberType, ok := enumType.Types[0].(*types.EnumMemberType); ok {
					targetTypeStr = memberType.EnumName
				} else {
					targetTypeStr = targetType.String()
				}
			} else {
				targetTypeStr = targetType.String()
			}
		} else {
			targetTypeStr = targetType.String()
		}
	} else {
		targetTypeStr = targetType.String()
	}

	return sourceTypeStr, targetTypeStr
}

// isEnumType checks if a type is an enum type
func (c *Checker) isEnumType(t types.Type) bool {
	if enumType, ok := t.(*types.UnionType); ok {
		if len(enumType.Types) > 0 {
			return types.IsEnumMemberType(enumType.Types[0])
		}
	}
	// Also check for specific enum member literal types
	return types.IsEnumMemberType(t)
}

// isGeneratorType checks if a type is a Generator type
func (c *Checker) isGeneratorType(t types.Type) bool {
	if instantiatedType, ok := t.(*types.InstantiatedType); ok {
		if instantiatedType.Generic != nil {
			return instantiatedType.Generic.Name == "Generator"
		}
	}
	return false
}

func (c *Checker) isAsyncGeneratorType(t types.Type) bool {
	if instantiatedType, ok := t.(*types.InstantiatedType); ok {
		if instantiatedType.Generic != nil {
			return instantiatedType.Generic.Name == "AsyncGenerator"
		}
	}
	return false
}

// isInAsyncContext checks if we're currently inside an async function or async generator
func (c *Checker) isInAsyncContext() bool {
	// Top level is async (TLA support) or we're inside an async function
	return c.functionNestingDepth == 0 || c.inAsyncFunction
}

// getNextAnonymousId generates a unique ID for anonymous classes
func (c *Checker) getNextAnonymousId() int {
	c.anonymousClassCounter++
	return c.anonymousClassCounter
}

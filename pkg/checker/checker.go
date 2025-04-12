package checker

import (
	"fmt"
	"paserati/pkg/errors" // Added import
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

const checkerDebug = false

func debugPrintf(format string, args ...interface{}) {
	if checkerDebug {
		fmt.Printf(format, args...)
	}
}

// Checker performs static type checking on the AST.
type Checker struct {
	program *parser.Program // Root AST node
	// TODO: Add Type Registry if needed
	env    *Environment           // Current type environment
	errors []errors.PaseratiError // Changed from []TypeError

	// --- NEW: Context for checking function bodies ---
	// Expected return type of the function currently being checked (set by explicit annotation).
	currentExpectedReturnType types.Type
	// List of types found in return statements within the current function (used for inference).
	currentInferredReturnTypes []types.Type
}

// NewChecker creates a new type checker.
func NewChecker() *Checker {
	return &Checker{
		env:    NewEnvironment(),         // Start with a global environment
		errors: []errors.PaseratiError{}, // Initialize with correct type
		// Initialize function context fields to nil/empty
		currentExpectedReturnType:  nil,
		currentInferredReturnTypes: nil,
	}
}

// Check analyzes the given program AST for type errors.
func (c *Checker) Check(program *parser.Program) []errors.PaseratiError {
	c.program = program
	c.errors = []errors.PaseratiError{} // Reset errors
	c.env = NewGlobalEnvironment()      // Start with a fresh global environment for this check
	globalEnv := c.env

	// --- Data Structures for Passes ---
	nodesProcessedPass1 := make(map[parser.Node]bool)   // Nodes handled in Pass 1 (Type Aliases)
	nodesProcessedPass2 := make(map[parser.Node]bool)   // Nodes handled in Pass 2 (Signatures/Vars)
	functionsToVisitBody := []*parser.FunctionLiteral{} // Function literals needing body check in Pass 3

	// --- Pass 1: Define ALL Type Aliases ---
	debugPrintf("\n// --- Checker - Pass 1: Defining Type Aliases ---\n")
	for _, stmt := range program.Statements {
		if aliasStmt, ok := stmt.(*parser.TypeAliasStatement); ok {
			debugPrintf("// [Checker Pass 1] Processing Type Alias: %s\n", aliasStmt.Name.Value)
			c.checkTypeAliasStatement(aliasStmt) // Uses c.env (globalEnv)
			nodesProcessedPass1[aliasStmt] = true
			nodesProcessedPass2[aliasStmt] = true // Also mark for Pass 2 skip
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
			initialSignature := c.resolveFunctionLiteralType(funcLit, globalEnv)
			if initialSignature == nil { // Handle resolution error
				initialSignature = &types.FunctionType{ // Default to Any signature on error
					ParameterTypes: make([]types.Type, len(funcLit.Parameters)),
					ReturnType:     types.Any,
				}
				for i := range initialSignature.ParameterTypes {
					initialSignature.ParameterTypes[i] = types.Any
				}
			}

			if !globalEnv.Define(name, initialSignature, false) { // Define with initial (maybe incomplete) signature
				c.addError(funcLit.Name, fmt.Sprintf("identifier '%s' already defined (hoisted)", name))
			}
			funcLit.SetComputedType(initialSignature) // Set initial type on node
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
		case *parser.LetStatement, *parser.ConstStatement, *parser.VarStatement:
			var varName *parser.Identifier
			var typeAnnotation parser.Expression
			var initializer parser.Expression
			isConst := false

			// Extract common fields
			switch specificNode := node.(type) {
			case *parser.LetStatement:
				varName = specificNode.Name
				typeAnnotation = specificNode.TypeAnnotation
				initializer = specificNode.Value
				debugPrintf("// [Checker Pass 2] Processing Let: %s\n", varName.Value)
			case *parser.ConstStatement:
				varName = specificNode.Name
				typeAnnotation = specificNode.TypeAnnotation
				initializer = specificNode.Value
				isConst = true
				debugPrintf("// [Checker Pass 2] Processing Const: %s\n", varName.Value)
			case *parser.VarStatement:
				varName = specificNode.Name
				typeAnnotation = specificNode.TypeAnnotation
				initializer = specificNode.Value
				debugPrintf("// [Checker Pass 2] Processing Var: %s\n", varName.Value)
			}

			var declaredType types.Type
			if typeAnnotation != nil {
				declaredType = c.resolveTypeAnnotation(typeAnnotation) // Use globalEnv implicitly
			}

			var preliminaryType types.Type = declaredType // Start with annotation type

			// Check initializer specifically for FunctionLiteral
			if funcLitInitializer, ok := initializer.(*parser.FunctionLiteral); ok {
				debugPrintf("// [Checker Pass 2] Variable '%s' initialized with FunctionLiteral\n", varName.Value)
				initialFuncSignature := c.resolveFunctionLiteralType(funcLitInitializer, globalEnv)
				if initialFuncSignature == nil { // Handle resolution error
					initialFuncSignature = &types.FunctionType{ // Default to Any signature on error
						ParameterTypes: make([]types.Type, len(funcLitInitializer.Parameters)),
						ReturnType:     types.Any,
					}
					for i := range initialFuncSignature.ParameterTypes {
						initialFuncSignature.ParameterTypes[i] = types.Any
					}
				}
				// Use function signature type if no annotation, or check compatibility if annotation exists
				if preliminaryType == nil {
					preliminaryType = initialFuncSignature
				} else {
					// TODO: Check if initialFuncSignature is assignable to declaredType?
					// For now, declaredType takes precedence if both exist.
				}
				funcLitInitializer.SetComputedType(initialFuncSignature) // Set initial type on the initializer node
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

		// Get the initial signature determined in Pass 2
		initialSignature := funcLit.GetComputedType()
		funcTypeSignature, ok := initialSignature.(*types.FunctionType)
		if !ok || funcTypeSignature == nil {
			debugPrintf("// [Checker Pass 3] ERROR: Could not get initial FunctionType for %s\n", funcNameForLog)
			// Maybe try resolving again? Or skip? Let's skip for now.
			c.addError(funcLit, fmt.Sprintf("internal checker error: failed to retrieve initial signature for %s", funcNameForLog))
			continue
		}

		// Save outer context & Set context for body check
		outerExpectedReturnType := c.currentExpectedReturnType
		outerInferredReturnTypes := c.currentInferredReturnTypes
		c.currentExpectedReturnType = funcTypeSignature.ReturnType // Use return type from initial signature
		c.currentInferredReturnTypes = nil
		if c.currentExpectedReturnType == nil {
			c.currentInferredReturnTypes = []types.Type{} // Allocate only if inference needed
		}

		// Create function's inner scope & define parameters
		originalEnv := c.env
		funcEnv := NewEnclosedEnvironment(originalEnv)
		c.env = funcEnv
		// Define parameters using the initial signature
		for i, paramNode := range funcLit.Parameters {
			if i < len(funcTypeSignature.ParameterTypes) {
				paramType := funcTypeSignature.ParameterTypes[i]
				if !funcEnv.Define(paramNode.Name.Value, paramType, false) {
					c.addError(paramNode.Name, fmt.Sprintf("duplicate parameter name: %s", paramNode.Name.Value))
				}
				paramNode.ComputedType = paramType // Set type on parameter node
			} else {
				debugPrintf("// [Checker Pass 3] ERROR: Param count mismatch for func '%s'\n", funcNameForLog)
			}
		}
		// Define function itself within its scope for recursion (using initial signature)
		if funcLit.Name != nil {
			funcEnv.Define(funcLit.Name.Value, funcTypeSignature, false) // Ignore error if already defined (e.g. hoisted)
		}

		// Visit Body
		c.visit(funcLit.Body) // Use funcEnv implicitly

		// Determine Final ACTUAL Return Type
		var actualReturnType types.Type
		if funcTypeSignature.ReturnType != nil { // Annotation existed
			actualReturnType = funcTypeSignature.ReturnType
		} else { // No annotation, infer
			if len(c.currentInferredReturnTypes) == 0 {
				actualReturnType = types.Undefined
			} else {
				actualReturnType = types.NewUnionType(c.currentInferredReturnTypes...)
			}
			debugPrintf("// [Checker Pass 3] Inferred return type for '%s': %s\n", funcNameForLog, actualReturnType.String())
		}

		// Create the FINAL FunctionType
		finalFuncType := &types.FunctionType{
			ParameterTypes: funcTypeSignature.ParameterTypes,
			ReturnType:     actualReturnType,
		}

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
	}
	debugPrintf("// --- Checker - Pass 3: Complete ---\n")

	// --- Pass 4: Final Check of Remaining Statements & Initializers ---
	debugPrintf("\n// --- Checker - Pass 4: Final Checks & Remaining Statements ---\n")
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
				debugPrintf("// [Checker Pass 4] Skipping already processed/visited node: %T\n", stmt)
				continue
			} else {
				debugPrintf("// [Checker Pass 4] Re-visiting Let/Const/Var for initializer check: %T\n", stmt)
			}
		}

		debugPrintf("// [Checker Pass 4] Visiting node: %T\n", stmt)

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
				debugPrintf("// [Checker Pass 4] Checking initializer for variable '%s'\n", varName.Value)
				c.visit(initializer) // Visit initializer expression
				computedInitializerType := initializer.GetComputedType()
				if computedInitializerType == nil {
					computedInitializerType = types.Any
				}

				// Get the variable's type defined in Pass 2 (might be Any)
				variableType, _, found := globalEnv.Resolve(varName.Value)
				if !found { // Should not happen
					debugPrintf("// [Checker Pass 4] ERROR: Variable '%s' not found in env during final check?\n", varName.Value)
					continue
				}

				// Perform assignability check using the type from env (e.g., Any or annotation)
				assignable := c.isAssignable(computedInitializerType, variableType)

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

				if !assignable {
					c.addError(initializer, fmt.Sprintf("cannot assign type '%s' to variable '%s' of type '%s'", computedInitializerType.String(), varName.Value, variableType.String()))
				}

				// --- FIX: Refine variable type in environment if no annotation ---
				if typeAnnotation == nil && found { // Check if ANNOTATION was nil
					// Widen literal types before updating environment, unless it's empty array
					var finalInferredType types.Type
					if isEmptyArrayAssignment {
						finalInferredType = computedInitializerType // Keep unknown[] type
					} else {
						finalInferredType = deeplyWidenType(computedInitializerType) // Use the deep widen helper
					}

					// Update the environment only if the refined type is different from the current one
					if variableType != finalInferredType {
						debugPrintf("// [Checker Pass 4] Refining type for '%s' (no annotation). Old: %s, New: %s\n", varName.Value, variableType.String(), finalInferredType.String())
						if !globalEnv.Update(varName.Value, finalInferredType) {
							debugPrintf("// [Checker Pass 4] WARNING: Failed env update refinement for '%s'\n", varName.Value)
						}
						// Also update the type on the name node itself for consistency
						varName.SetComputedType(finalInferredType)
					} else {
						debugPrintf("// [Checker Pass 4] Type for '%s' already refined to %s. No update needed.\n", varName.Value, variableType.String())
					}
				}
				// --- END FIX ---
			}

		case *parser.ExpressionStatement:
			debugPrintf("// [Checker Pass 4] Visiting ExpressionStatement\n")
			c.visit(node.Expression) // Visit the expression (e.g., calls, assignments)

		// TODO: Handle other top-level statement types if necessary
		default:
			debugPrintf("// [Checker Pass 4] Visiting unhandled statement type %T\n", node)
			c.visit(node) // Fallback visit? Might be unnecessary
		}
	}
	debugPrintf("// --- Checker - Pass 4: Complete ---\n")

	return c.errors
}

// --- Helper Functions ---

// resolveTypeAnnotation converts a parser node representing a type annotation
// into a types.Type representation.
func (c *Checker) resolveTypeAnnotation(node parser.Expression) types.Type {
	if node == nil {
		// No annotation provided, perhaps default to any or handle elsewhere?
		// For now, returning nil might be okay, caller decides default.
		return nil
	}

	// Dispatch based on the structure of the type expression node
	switch node := node.(type) {
	case *parser.Identifier:
		// --- ADDED: Check if node or node.Value is nil ---
		if node == nil || node.Value == "" {
			debugPrintf("// [Checker resolveTypeAnno Ident] ERROR: Node (%p) or Node.Value is nil/empty!\n", node)
			return nil // Return nil early if node is bad
		}
		debugPrintf("// [Checker resolveTypeAnno Ident] Processing identifier: '%s'\n", node.Value)
		// --- END ADDED ---

		// --- UPDATED: Prioritize alias resolution ---
		// 1. Attempt to resolve as a type alias in the environment
		resolvedAlias, found := c.env.ResolveType(node.Value)
		if found {
			debugPrintf("// [Checker resolveTypeAnno Ident] Resolved '%s' as alias: %T\n", node.Value, resolvedAlias) // ADDED DEBUG
			return resolvedAlias                                                                                      // Successfully resolved as an alias
		}
		debugPrintf("// [Checker resolveTypeAnno Ident] '%s' not found as alias, checking primitives...\n", node.Value) // ADDED DEBUG

		// 2. If not found as an alias, check against known primitive type names
		switch node.Value {
		case "number":
			debugPrintf("// [Checker resolveTypeAnno Ident] Matched primitive: number\n") // ADDED DEBUG
			return types.Number                                                           // Returns the package-level variable 'types.Number'
		case "string":
			debugPrintf("// [Checker resolveTypeAnno Ident] Matched primitive: string\n") // ADDED DEBUG
			return types.String
		case "boolean":
			return types.Boolean
		case "null":
			return types.Null
		case "undefined":
			return types.Undefined
		case "any":
			return types.Any
		case "unknown":
			return types.Unknown
		case "never":
			return types.Never
		case "void":
			return types.Void
		default:
			// 3. If neither alias nor primitive, it's an unknown type name
			debugPrintf("// [Checker resolveTypeAnno Ident] Primitive check failed for '%s', reporting error.\n", node.Value) // ADDED DEBUG
			// Use the Identifier node itself for error reporting
			c.addError(node, fmt.Sprintf("unknown type name: %s", node.Value))
			return nil // Indicate error
		}

	case *parser.UnionTypeExpression: // Added
		leftType := c.resolveTypeAnnotation(node.Left)
		rightType := c.resolveTypeAnnotation(node.Right)

		if leftType == nil || rightType == nil {
			// Error occurred resolving one of the sides
			return nil
		}

		// --- UPDATED: Use NewUnionType constructor ---
		// This handles flattening and duplicate removal automatically.
		return types.NewUnionType(leftType, rightType)

	// --- NEW: Handle ArrayTypeExpression ---
	case *parser.ArrayTypeExpression:
		elemType := c.resolveTypeAnnotation(node.ElementType)
		if elemType == nil {
			return nil // Error resolving element type
		}
		arrayType := &types.ArrayType{ElementType: elemType}
		// No need to set computed type on the type annotation node itself
		return arrayType

	// --- NEW: Handle Literal Type Nodes ---
	case *parser.StringLiteral:
		return &types.LiteralType{Value: vm.String(node.Value)}
	case *parser.NumberLiteral:
		return &types.LiteralType{Value: vm.Number(node.Value)}
	case *parser.BooleanLiteral:
		return &types.LiteralType{Value: vm.Bool(node.Value)}
	case *parser.NullLiteral:
		return types.Null
	case *parser.UndefinedLiteral:
		return types.Undefined
	// --- End Literal Type Nodes ---

	// --- NEW: Handle FunctionTypeExpression --- <<<
	case *parser.FunctionTypeExpression:
		return c.resolveFunctionTypeSignature(node)

	default:
		// If we get here, the parser created a node type that resolveTypeAnnotation doesn't handle yet.
		c.addError(node, fmt.Sprintf("unsupported type annotation node: %T", node))
		return nil // Indicate error
	}
}

// --- NEW: Helper to resolve FunctionTypeExpression nodes ---
func (c *Checker) resolveFunctionTypeSignature(node *parser.FunctionTypeExpression) types.Type {
	paramTypes := []types.Type{}
	for _, paramNode := range node.Parameters {
		paramType := c.resolveTypeAnnotation(paramNode)
		if paramType == nil {
			// Error should have been added by resolveTypeAnnotation
			return nil // Indicate error by returning nil
		}
		paramTypes = append(paramTypes, paramType)
	}

	returnType := c.resolveTypeAnnotation(node.ReturnType)
	if returnType == nil {
		// Error should have been added by resolveTypeAnnotation
		return nil // Indicate error by returning nil
	}

	// Construct the internal FunctionType representation
	return &types.FunctionType{ParameterTypes: paramTypes, ReturnType: returnType}
}

// isAssignable checks if a value of type `source` can be assigned to a variable
// of type `target`.
// TODO: Expand significantly for structural typing, unions, intersections etc.
func (c *Checker) isAssignable(source, target types.Type) bool {
	if source == nil || target == nil {
		// Cannot determine assignability if one type is unknown/error
		// Defaulting to true might hide errors, false seems safer.
		return false
	}

	// Basic rules:
	if target == types.Any || source == types.Any {
		return true // Any accepts anything, anything goes into Any
	}

	if target == types.Unknown {
		return true // Anything can be assigned to Unknown
	}
	if source == types.Unknown {
		// Unknown can only be assigned to Unknown or Any (already handled)
		return target == types.Unknown
	}

	if source == types.Never {
		return true // Never type is assignable to anything
	}

	// Check for identical types (using pointer equality for primitives)
	if source == target {
		return true
	}

	// --- NEW: Union Type Handling ---
	sourceUnion, sourceIsUnion := source.(*types.UnionType)
	targetUnion, targetIsUnion := target.(*types.UnionType)

	if targetIsUnion {
		// Assigning TO a union: Source must be assignable to at least one type in the target union.
		if sourceIsUnion {
			// Assigning UNION to UNION (S_union to T_union):
			// Every type in S_union must be assignable to at least one type in T_union.
			for _, sType := range sourceUnion.Types {
				assignableToOneInTarget := false
				for _, tType := range targetUnion.Types {
					if c.isAssignable(sType, tType) {
						assignableToOneInTarget = true
						break
					}
				}
				if !assignableToOneInTarget {
					return false // Found a type in source union not assignable to any in target union
				}
			}
			return true // All types in source union were assignable to the target union
		} else {
			// Assigning NON-UNION to UNION (S to T_union):
			// S must be assignable to at least one type in T_union.
			for _, tType := range targetUnion.Types {
				if c.isAssignable(source, tType) {
					return true // Found a compatible type in the union
				}
			}
			return false // Source not assignable to any type in the target union
		}
	} else if sourceIsUnion {
		// Assigning FROM a union TO a non-union (S_union to T):
		// Every type in S_union must be assignable to T.
		for _, sType := range sourceUnion.Types {
			if !c.isAssignable(sType, target) {
				return false // Found a type in the source union not assignable to the target
			}
		}
		return true // All types in source union were assignable to target
	}

	// --- End Union Type Handling ---

	// --- NEW: Literal Type Handling ---
	sourceLiteral, sourceIsLiteral := source.(*types.LiteralType)
	targetLiteral, targetIsLiteral := target.(*types.LiteralType)

	if sourceIsLiteral && targetIsLiteral {
		// Assigning LiteralType to LiteralType: Values must be strictly equal
		// Use vm.valuesEqual (unexported) logic for now.
		// Types must match AND values must match.
		if sourceLiteral.Value.Type != targetLiteral.Value.Type {
			return false
		}
		// Use the existing loose equality check from VM package
		// as types are already confirmed to be the same.
		// Need to export valuesEqual or replicate logic.
		// Let's replicate the core logic here for simplicity for now.
		switch sourceLiteral.Value.Type {
		case vm.TypeNull:
			return true // null === null
		case vm.TypeUndefined:
			return true // undefined === undefined
		case vm.TypeBool:
			return vm.AsBool(sourceLiteral.Value) == vm.AsBool(targetLiteral.Value)
		case vm.TypeNumber:
			return vm.AsNumber(sourceLiteral.Value) == vm.AsNumber(targetLiteral.Value)
		case vm.TypeString:
			return vm.AsString(sourceLiteral.Value) == vm.AsString(targetLiteral.Value)
		default:
			return false // Literal types cannot be functions/closures/etc.
		}
		// return vm.valuesEqual(sourceLiteral.Value, targetLiteral.Value) // If vm.valuesEqual was exported
	} else if sourceIsLiteral {
		// Assigning LiteralType TO Non-LiteralType (target):
		// Check if the literal's underlying primitive type is assignable to the target.
		// e.g., LiteralType{"hello"} -> string (true)
		// e.g., LiteralType{123} -> number (true)
		// e.g., LiteralType{true} -> boolean (true)
		// e.g., LiteralType{"hello"} -> number (false)
		var underlyingPrimitiveType types.Type
		switch sourceLiteral.Value.Type {
		case vm.TypeString:
			underlyingPrimitiveType = types.String
		case vm.TypeNumber:
			underlyingPrimitiveType = types.Number
		case vm.TypeBool:
			underlyingPrimitiveType = types.Boolean
		// Cannot have literal types for null/undefined/functions/etc.
		default:
			return false // Or maybe Any/Unknown?
		}
		// Check assignability between the underlying primitive and the target
		return c.isAssignable(underlyingPrimitiveType, target)
	} else if targetIsLiteral {
		// Assigning Non-LiteralType (source) TO LiteralType:
		// This is generally only possible if the source is also a literal type
		// with the exact same value, which is covered by the first case (sourceIsLiteral && targetIsLiteral).
		// Or if the source is 'any'/'unknown' (already handled).
		// Or if the source is 'never' (already handled).
		return false
	}
	// --- End Literal Type Handling ---

	// --- NEW: Array Type Assignability ---
	sourceArray, sourceIsArray := source.(*types.ArrayType)
	targetArray, targetIsArray := target.(*types.ArrayType)

	if sourceIsArray && targetIsArray {
		// Both are arrays. Check if source element type is assignable to target element type.
		// This is a basic covariance check. Stricter checks might be needed later.
		if sourceArray.ElementType == nil || targetArray.ElementType == nil {
			// If either element type is unknown (shouldn't happen?), consider it not assignable for safety.
			return false
		}
		return c.isAssignable(sourceArray.ElementType, targetArray.ElementType)
	}

	// --- End Array Type Handling ---

	// --- NEW: Function Type Assignability ---
	sourceFunc, sourceIsFunc := source.(*types.FunctionType)
	targetFunc, targetIsFunc := target.(*types.FunctionType)

	if targetIsFunc {
		if sourceIsFunc {
			// Assigning Function to Function

			// Check Arity
			if len(sourceFunc.ParameterTypes) != len(targetFunc.ParameterTypes) {
				return false // Arity mismatch
			}

			// Check Parameter Types (Contravariance - target param assignable to source param)
			// For simplicity now, let's check invariance: source param assignable to target param
			for i, targetParamType := range targetFunc.ParameterTypes {
				sourceParamType := sourceFunc.ParameterTypes[i]
				// if !c.isAssignable(targetParamType, sourceParamType) { // Contravariant check
				if !c.isAssignable(sourceParamType, targetParamType) { // Invariant check (simpler)
					return false // Parameter type mismatch
				}
			}

			// Check Return Type (Covariance - source return assignable to target return)
			if !c.isAssignable(sourceFunc.ReturnType, targetFunc.ReturnType) {
				return false // Return type mismatch
			}

			// All checks passed
			return true
		} else {
			// Assigning Non-Function to Function Target: Generally false
			// (Unless source is Any/Unknown/Never, handled earlier)
			return false
		}
	}
	// --- End Function Type Handling ---

	// TODO: Handle null/undefined assignability based on strict flags later.
	// For now, let's be strict unless target is Any/Unknown/Union.
	if source == types.Null && target != types.Null { // Allow null -> T | null
		return false
	}
	if source == types.Undefined && target != types.Undefined { // Allow undefined -> T | undefined
		return false
	}

	// TODO: Add structural checks for objects/arrays
	// TODO: Add checks for function type compatibility
	// TODO: Add checks for intersections

	// Default: not assignable
	return false
}

// visit is the main dispatch method for AST traversal (Visitor pattern lite).
func (c *Checker) visit(node parser.Node) {
	if node == nil {
		return
	}
	debugPrintf("// [Checker Visit Enter] Node: %T, Env: %p\n", node, c.env)

	switch node := node.(type) {
	case *parser.Program:
		for _, stmt := range node.Statements {
			c.visit(stmt)
		}

	case *parser.TypeAliasStatement:
		c.checkTypeAliasStatement(node)

	case *parser.ExpressionStatement:
		c.visit(node.Expression)

	case *parser.LetStatement:
		// <<< Capture pointers at entry >>>
		letNodePtr := node
		letNamePtr := node.Name
		letValuePtr := node.Value
		var nameValueStr string
		if letNamePtr != nil {
			nameValueStr = letNamePtr.Value
		} else {
			nameValueStr = "<nil_name>"
		}
		debugPrintf("// [Checker LetStmt Entry] NodePtr: %p, NamePtr: %p (%s), ValuePtr: %p\n", letNodePtr, letNamePtr, nameValueStr, letValuePtr)

		// --- UPDATED: Handle LetStatement ---
		// 1. Handle Type Annotation (if present)
		var declaredType types.Type
		if node.TypeAnnotation != nil {
			declaredType = c.resolveTypeAnnotation(node.TypeAnnotation)
		} else {
			declaredType = nil // Indicates type inference is needed
		}

		// --- FIX V2 for recursive functions assigned to variables (Needs review with new logic) ---
		// Determine a preliminary type for the initializer if it's a function literal.
		var preliminaryInitializerType types.Type = nil
		if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
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
			preliminaryInitializerType = &types.FunctionType{
				ParameterTypes: prelimParamTypes,
				ReturnType:     prelimReturnType,
			}
		} // TODO: Handle ArrowFunctionLiteral similarly if needed

		// Temporarily define the variable name in the current scope before visiting the value.
		// Use preliminary func type if available, then declared type, else Any.
		tempType := preliminaryInitializerType
		if tempType == nil {
			tempType = declaredType // Use annotation if no prelim func type
		}
		if tempType == nil {
			tempType = types.Any // Fallback to Any
		}
		if !c.env.Define(node.Name.Value, tempType, false) {
			// If Define fails here, it's a true redeclaration error.
			c.addError(node.Name, fmt.Sprintf("variable '%s' already declared in this scope", node.Name.Value))
		}
		debugPrintf("// [Checker LetStmt] Temp Define '%s' as: %s\n", node.Name.Value, tempType.String()) // DEBUG
		// --- END FIX V2 ---

		// 2. Handle Initializer (if present)
		var computedInitializerType types.Type
		if node.Value != nil { // Use node.Value directly
			c.visit(node.Value) // Visits the FunctionLiteral

			// <<< IMMEDIATE CHECK AFTER RETURN >>>
			if node.Name == nil {
				// Use the name string captured at the START of the Let visit
				panic(fmt.Sprintf("PANIC CHECKER: node.Name for LetStatement '%s' became nil IMMEDIATELY AFTER visiting the value node! NodePtr: %p", nameValueStr, node))
			}
			// <<< END IMMEDIATE CHECK >>>

			// <<< Log pointers AFTER visit using captured letNodePtr >>>
			currentNamePtr := letNodePtr.Name // Get current Name pointer
			var currentNameValueStr string
			if currentNamePtr != nil {
				currentNameValueStr = currentNamePtr.Value
			} else {
				currentNameValueStr = "<nil_name>"
			}
			debugPrintf("// [Checker LetStmt Post-Visit] NodePtr: %p, NamePtr: %p (%s), ValuePtr: %p\n", letNodePtr, currentNamePtr, currentNameValueStr, letNodePtr.Value)

			// Double check node.Value didn't become nil
			if node.Value == nil {
				// This panic shouldn't trigger if the one above didn't, but keep for safety
				panic(fmt.Sprintf("PANIC CHECKER: node.Value for LetStatement '%s' became nil AFTER visiting the value node!", currentNameValueStr)) // Use currentNameValueStr
			}

			computedInitializerType = node.Value.GetComputedType()                                                                                                                        // Should be safe if we passed checks
			debugPrintf("// [Checker LetStmt] '%s': computedInitializerType from node.Value (%T): %T (%v)\n", nameValueStr, node.Value, computedInitializerType, computedInitializerType) // ADDED DEBUG

			// Use the SAFE nameValueStr (captured at entry) for logging
			// debugPrintf("// [Checker LetStmt] '%s': Got computedInitializerType: %T\n", nameValueStr, computedInitializerType) // Keep commented
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
				assignable := c.isAssignable(computedInitializerType, declaredType)

				// --- SPECIAL CASE: Allow assignment of [] (unknown[]) to T[] ---
				isEmptyArrayAssignment := false
				if _, isTargetArray := declaredType.(*types.ArrayType); isTargetArray { // Use blank identifier
					if sourceArrayType, isSourceArray := computedInitializerType.(*types.ArrayType); isSourceArray {
						if sourceArrayType.ElementType == types.Unknown {
							isEmptyArrayAssignment = true
						}
					}
				}
				// --- END SPECIAL CASE ---

				if !assignable && !isEmptyArrayAssignment { // Report error only if not normally assignable AND not the empty array case
					c.addError(node.Value, fmt.Sprintf("cannot assign type '%s' to variable '%s' of type '%s'", computedInitializerType.String(), node.Name.Value, finalVariableType.String()))
				}
			} // else: No initializer, declaredType is fine
		} else {
			// --- No annotation: Infer type ---
			if computedInitializerType != nil {
				if _, isLiteral := computedInitializerType.(*types.LiteralType); isLiteral {
					finalVariableType = types.GetWidenedType(computedInitializerType)
					debugPrintf("// [Checker LetStmt] '%s': Inferred final type (widened literal): %s (Go Type: %T)\n", nameValueStr, finalVariableType.String(), finalVariableType) // Use nameValueStr
				} else {
					finalVariableType = computedInitializerType                                                                                                                                // finalVariableType = () => undefined
					debugPrintf("// [Checker LetStmt] '%s': Assigned finalVariableType (direct non-literal): %s (Go Type: %T)\n", nameValueStr, finalVariableType.String(), finalVariableType) // Use nameValueStr
				}
			} else {
				finalVariableType = types.Undefined
				debugPrintf("// [Checker LetStmt] '%s': Inferred final type (no initializer): %s (Go Type: %T)\n", nameValueStr, finalVariableType.String(), finalVariableType) // Use nameValueStr
			}
		}

		// 4. UPDATE variable type in the current environment with the final type
		// We use Define again which will overwrite the temporary type.
		// --- DEBUG: Check types before and after update ---
		currentType, _, _ := c.env.Resolve(node.Name.Value)
		debugPrintf("// [Checker LetStmt] Updating '%s'. Current type: %s, Final type: %s\n", node.Name.Value, currentType.String(), finalVariableType.String())
		// --- END DEBUG ---
		if !c.env.Update(node.Name.Value, finalVariableType) { // Use finalVariableType
			debugPrintf("// [Checker LetStmt] WARNING: Update failed unexpectedly for '%s'\n", node.Name.Value)
			// This might indicate the symbol wasn't found, which shouldn't happen here
			// If it truly needs to be defined (e.g., recursive case refinement needed?),
			// we might need more complex logic, but Update should work for simple inference.
		}
		// --- DEBUG: Check type after update ---
		updatedType, _, _ := c.env.Resolve(node.Name.Value)
		debugPrintf("// [Checker LetStmt] Updated '%s'. Type after update: %s\n", node.Name.Value, updatedType.String())
		// --- END DEBUG ---

		// Set computed type on the Name Identifier node itself
		if node.Name == nil {
			panic(fmt.Sprintf("PANIC CHECKER: node.Name is nil before final SetComputedType for %s", nameValueStr))
		}
		node.Name.SetComputedType(finalVariableType)

	case *parser.ConstStatement:
		// --- UPDATED: Handle ConstStatement ---
		// 1. Handle Type Annotation (if present)
		var declaredType types.Type
		if node.TypeAnnotation != nil {
			declaredType = c.resolveTypeAnnotation(node.TypeAnnotation)
		} else {
			declaredType = nil // Indicates type inference is needed
		}

		// 2. Handle Initializer (Must be present for const)
		var computedInitializerType types.Type
		if node.Value != nil {
			c.visit(node.Value)                                    // Compute type of the initializer
			computedInitializerType = node.Value.GetComputedType() // <<< USE NODE METHOD
		} else {
			// Constants MUST be initialized
			c.addError(node.Value, fmt.Sprintf("const declaration '%s' must be initialized", node.Name.Value))
			computedInitializerType = types.Any // Assign Any to prevent further cascading errors downstream
		}

		// 3. Determine the final type and check assignment errors
		var finalType types.Type // Renamed from finalVariableType for clarity

		if declaredType != nil {
			// --- If annotation exists, constant type IS the annotation ---
			finalType = declaredType

			// Check if initializer is assignable to the declared type for error reporting
			assignable := c.isAssignable(computedInitializerType, declaredType)

			// --- SPECIAL CASE: Allow assignment of [] (unknown[]) to T[] ---
			isEmptyArrayAssignment := false
			if _, isTargetArray := declaredType.(*types.ArrayType); isTargetArray { // Use blank identifier
				if sourceArrayType, isSourceArray := computedInitializerType.(*types.ArrayType); isSourceArray {
					if sourceArrayType.ElementType == types.Unknown {
						isEmptyArrayAssignment = true
					}
				}
			}
			// --- END SPECIAL CASE ---

			if !assignable && !isEmptyArrayAssignment { // Report error only if not normally assignable AND not the empty array case
				c.addError(node.Value, fmt.Sprintf("cannot assign type '%s' to constant '%s' of type '%s'", computedInitializerType.String(), node.Name.Value, finalType.String()))
			}
		} else {
			// --- No annotation: Infer type ---
			// computedInitializerType should not be nil here due to const requirement check above

			// <<< MODIFIED: Only widen literal types >>>
			if _, isLiteral := computedInitializerType.(*types.LiteralType); isLiteral {
				finalType = types.GetWidenedType(computedInitializerType)
			} else {
				// Use the computed type directly for non-literals (functions, arrays, etc.)
				finalType = computedInitializerType
			}
		}

		// 4. Define variable in the current environment
		if !c.env.Define(node.Name.Value, finalType, true) {
			c.addError(node.Name, fmt.Sprintf("constant '%s' already declared in this scope", node.Name.Value))
		}
		// Set computed type on the Name Identifier node itself
		node.Name.SetComputedType(finalType) // <<< USE NODE METHOD

	case *parser.ReturnStatement:
		// --- UPDATED: Handle ReturnStatement ---
		var actualReturnType types.Type = types.Undefined // Default if no return value
		if node.ReturnValue != nil {
			c.visit(node.ReturnValue)
			actualReturnType = node.ReturnValue.GetComputedType() // <<< USE NODE METHOD
			if actualReturnType == nil {
				actualReturnType = types.Any
			} // Handle nil from potential error
		}

		// Check against expected type if available
		if c.currentExpectedReturnType != nil {
			if !c.isAssignable(actualReturnType, c.currentExpectedReturnType) {
				msg := fmt.Sprintf("cannot return value of type %s from function expecting %s",
					actualReturnType, c.currentExpectedReturnType)
				// Report the error at the return value expression node
				c.addError(node.ReturnValue, msg)
			}
		}

		// Add to inferred types
		c.currentInferredReturnTypes = append(c.currentInferredReturnTypes, actualReturnType)

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
				funcType := c.resolveFunctionLiteralType(funcLit, c.env)
				if funcType == nil {
					debugPrintf("// [Checker Block Hoisting] WARNING: Failed to resolve signature for hoisted func '%s'. Defining as Any.\n", name)
					if !c.env.Define(name, types.Any, false) {
						c.addError(funcLit.Name, fmt.Sprintf("identifier '%s' already defined in this block scope", name))
					}
					continue
				}

				// Define the function in the block environment
				if !c.env.Define(name, funcType, false) {
					// Duplicate definition error
					c.addError(funcLit.Name, fmt.Sprintf("identifier '%s' already defined in this block scope", name))
				}

				// Set the computed type on the FunctionLiteral node itself NOW.
				funcLit.SetComputedType(funcType)
				debugPrintf("// [Checker Block Hoisting] Hoisted and defined func '%s' with type: %s\n", name, funcType.String())
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

	case *parser.StringLiteral:
		literalType := &types.LiteralType{Value: vm.String(node.Value)}
		node.SetComputedType(literalType) // <<< USE NODE METHOD

	case *parser.BooleanLiteral:
		literalType := &types.LiteralType{Value: vm.Bool(node.Value)}
		// Treat boolean literals as literal types during checking
		node.SetComputedType(literalType) // <<< USE NODE METHOD

	case *parser.NullLiteral:
		node.SetComputedType(types.Null)

	case *parser.UndefinedLiteral:
		node.SetComputedType(types.Undefined)

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

		typ, isConst, found := c.env.Resolve(node.Value) // Use node.Value directly; UPDATED TO 3 VARS
		if !found {
			debugPrintf("// [Checker Debug] visit(Identifier): '%s' not found in env %p\n", node.Value, c.env) // DEBUG
			c.addError(node, fmt.Sprintf("undefined variable: %s", node.Value))
			// Set computed type if node itself is not nil (already checked)
			node.SetComputedType(types.Any) // Set to Any on error?
		} else {
			// --- DEBUG: Log raw type value immediately after resolve ---
			// debugPrintf("// [Checker Debug] Identifier: Resolved type for '%s': Ptr=%p, Value=%#v\n", node.Value, typ, typ) // Re-commented
			// --- END DEBUG ---

			debugPrintf("// [Checker Debug] visit(Identifier): '%s' found in env %p, type: %s\n", node.Value, c.env, typ.String()) // DEBUG - Uncommented

			// node is guaranteed non-nil here
			node.SetComputedType(typ)
			// --- DEBUG: Explicit panic before return ---
			// panic(fmt.Sprintf("Intentional panic after setting type for Identifier '%s'", node.Value))
			// --- END DEBUG ---

			// <<< ADD CONST CHECK FOR IDENTIFIER NODE >>>
			// Store the const status on the identifier node itself for later use in assignment checks.
			node.IsConstant = isConst // Re-enabled
		}

	case *parser.PrefixExpression:
		// --- UPDATED: Handle PrefixExpression ---
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
			// --- END NEW ---
			default:
				c.addError(node, fmt.Sprintf("unsupported prefix operator: %s", node.Operator))
			}
		} // else: Error might have occurred visiting operand, or type is nil.
		node.SetComputedType(resultType)

	case *parser.InfixExpression:
		// --- UPDATED: Handle InfixExpression ---
		c.visit(node.Left)
		c.visit(node.Right)
		leftType := node.Left.GetComputedType()
		rightType := node.Right.GetComputedType()

		if leftType == nil {
			leftType = types.Any
		}
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
				} else if widenedLeftType == types.String && widenedRightType == types.String {
					resultType = types.String
					// <<< NEW: Handle String + Number Coercion >>>
				} else if (widenedLeftType == types.String && widenedRightType == types.Number) ||
					(widenedLeftType == types.Number && widenedRightType == types.String) {
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
				} else {
					// Operands are not numbers (and not 'any'). This is a type error.
					c.addError(node, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, widenedLeftType.String(), widenedRightType.String()))
					// Keep resultType = types.Any (default)
				}
			// --- END NEW ---

			case "<", ">", "<=", ">=":
				if isAnyOperand {
					resultType = types.Any // Comparison with any results in any? Or boolean? Let's try Any first.
					// Alternatively: resultType = types.Boolean (safer, result is always boolean)
				} else if widenedLeftType == types.Number && widenedRightType == types.Number {
					resultType = types.Boolean
				} else {
					c.addError(node.Right, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, widenedLeftType.String(), widenedRightType.String()))
					resultType = types.Boolean // Comparison errors still result in boolean
				}
			case "==", "!=", "===", "!==":
				// Comparison always results in boolean, even with 'any'
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
			default:
				debugPrintf("// [Checker Infix Pre-Check] Proceeding with operator: %s\n", node.Operator)
				c.addError(node.Right, fmt.Sprintf("unsupported infix operator: %s", node.Operator))
			}
		} // else: Error already reported during operand check or types were nil

		debugPrintf("// [Checker Infix] Node: %p (%s), Determined ResultType: %T (%v)\n", node, node.Operator, resultType, resultType)
		node.SetComputedType(resultType)

	case *parser.IfExpression:
		// --- UPDATED: Handle IfExpression ---
		// 1. Check Condition
		c.visit(node.Condition)

		// 2. Check Consequence block
		c.visit(node.Consequence)

		// 3. Check Alternative block (if it exists)
		if node.Alternative != nil {
			c.visit(node.Alternative)
		}

		// 4. Determine overall type (tricky! depends on return/break/continue)
		// For now, if expressions don't have a value themselves (unless ternary)
		// They control flow. Let's assign Void for now, representing no value produced by the if itself.
		// A more advanced checker might determine if both branches *must* return/throw,
		// or compute a union type of the last expressions if they are treated as values.
		node.SetComputedType(types.Void) // Use checker's method

	case *parser.TernaryExpression:
		// --- NEW: Handle TernaryExpression ---
		c.visit(node.Condition)
		// TODO: Check if condition is boolean?

		c.visit(node.Consequence)
		c.visit(node.Alternative)

		consType := node.Consequence.GetComputedType()
		altType := node.Alternative.GetComputedType()

		// Handle nil types from potential errors during visit
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
		// 1. Resolve explicit annotations FIRST to get the signature contract.
		//    Use resolveFunctionLiteralType helper, but pass the *current* env.
		//    This gives us the expected parameter types and *potential* return type.
		resolvedSignature := c.resolveFunctionLiteralType(node, c.env) // Pass c.env
		if resolvedSignature == nil {
			// Error resolving annotations (e.g., unknown type name)
			// Error should have been added by resolve helper.
			// Create a dummy Any signature to proceed safely.
			paramTypes := make([]types.Type, len(node.Parameters))
			for i := range paramTypes {
				paramTypes[i] = types.Any
			}
			resolvedSignature = &types.FunctionType{ParameterTypes: paramTypes, ReturnType: types.Any}
			// Set dummy type on node immediately to prevent nil checks later if we proceeded?
			// No, let's calculate the final type below and set it once.
		}

		// 2. Save outer return context
		outerExpectedReturnType := c.currentExpectedReturnType
		outerInferredReturnTypes := c.currentInferredReturnTypes

		// 3. Set context for BODY CHECK based ONLY on explicit return annotation.
		//    resolvedSignature.ReturnType can be nil if not annotated.
		c.currentExpectedReturnType = resolvedSignature.ReturnType // Use ReturnType from resolved signature
		c.currentInferredReturnTypes = nil                         // Reset inferred list for this function body
		if c.currentExpectedReturnType == nil {                    // Allocate ONLY if inference is actually needed
			c.currentInferredReturnTypes = []types.Type{}
		}

		// 4. Create function's inner scope & define parameters using resolved signature param types
		funcNameForLog := "<anonymous>"
		if node.Name != nil {
			funcNameForLog = node.Name.Value
		}
		debugPrintf("// [Checker Visit FuncLit] Creating INNER scope for '%s'. Current Env: %p\n", funcNameForLog, c.env)
		originalEnv := c.env
		funcEnv := NewEnclosedEnvironment(originalEnv)
		c.env = funcEnv
		for i, paramNode := range node.Parameters { // Iterate over parser nodes
			if i < len(resolvedSignature.ParameterTypes) { // Safety check using resolved signature
				paramType := resolvedSignature.ParameterTypes[i]
				if !funcEnv.Define(paramNode.Name.Value, paramType, false) {
					c.addError(paramNode.Name, fmt.Sprintf("duplicate parameter name: %s", paramNode.Name.Value))
				}
				// Set computed type on the Parameter node itself
				paramNode.ComputedType = paramType
			} else {
				// Mismatch between AST params and resolved signature params - internal error?
				debugPrintf("// [Checker FuncLit Visit] ERROR: Mismatch in param count for func '%s'\n", funcNameForLog)
			}
		}

		// --- Function name self-definition for recursion (if named) ---
		// Hoisting handles top-level/block-level, but let/const needs this.
		if node.Name != nil {
			// Re-use the resolvedSignature for the temporary definition
			// (ReturnType might still be nil here if not annotated)
			tempFuncTypeForRecursion := &types.FunctionType{
				ParameterTypes: resolvedSignature.ParameterTypes,
				ReturnType:     resolvedSignature.ReturnType, // Use potentially nil return type
			}
			if !funcEnv.Define(node.Name.Value, tempFuncTypeForRecursion, false) {
				// This might happen if a param has the same name - parser should likely prevent this
				c.addError(node.Name, fmt.Sprintf("function name '%s' conflicts with a parameter", node.Name.Value))
			}
		}
		// --- END Function name self-definition ---

		// 5. Visit Body
		c.visit(node.Body)

		// 6. Determine Final ACTUAL Return Type of the function body
		var actualReturnType types.Type
		if resolvedSignature.ReturnType != nil {
			// Annotation exists, use that as the final actual type.
			// Checks against this type happened during ReturnStatement visits.
			actualReturnType = resolvedSignature.ReturnType
		} else {
			// No annotation, INFER the return type from collected returns.
			if len(c.currentInferredReturnTypes) == 0 {
				actualReturnType = types.Undefined // No returns -> undefined
			} else {
				// Use NewUnionType to combine inferred return types
				actualReturnType = types.NewUnionType(c.currentInferredReturnTypes...)
			}
			debugPrintf("// [Checker FuncLit Visit] Inferred return type for '%s': %s\n", funcNameForLog, actualReturnType.String())
		}

		// --- Update self-definition if name existed and return type was inferred ---
		if node.Name != nil && resolvedSignature.ReturnType == nil {
			finalFuncTypeForRecursion := &types.FunctionType{
				ParameterTypes: resolvedSignature.ParameterTypes,
				ReturnType:     actualReturnType, // Use the inferred type now
			}
			// Update the function's own entry in its scope
			if !funcEnv.Update(node.Name.Value, finalFuncTypeForRecursion) {
				debugPrintf("// [Checker FuncLit Visit] WARNING: Failed to update self-definition for '%s'\n", node.Name.Value)
			}
		}
		// --- END Update self-definition ---

		// 7. Create the FINAL FunctionType representing this literal
		finalFuncType := &types.FunctionType{
			ParameterTypes: resolvedSignature.ParameterTypes, // Use types from annotation/defaults
			ReturnType:     actualReturnType,                 // Use the explicit or inferred return type
		}

		// 8. *** ALWAYS Set the Computed Type on the FunctionLiteral node ***
		debugPrintf("// [Checker FuncLit Visit] SETTING final computed type for '%s': %s\n", funcNameForLog, finalFuncType.String())
		node.SetComputedType(finalFuncType) // <<< THIS IS THE KEY FIX

		// 9. Restore outer environment and context
		debugPrintf("// [Checker Visit FuncLit] Exiting '%s'. Restoring Env: %p (from current %p)\n", funcNameForLog, originalEnv, c.env)
		c.env = originalEnv
		c.currentExpectedReturnType = outerExpectedReturnType
		c.currentInferredReturnTypes = outerInferredReturnTypes

	case *parser.ArrowFunctionLiteral:
		// --- UPDATED: Handle ArrowFunctionLiteral (Similar to FunctionLiteral) ---
		// 1. Save outer context
		outerExpectedReturnType := c.currentExpectedReturnType
		outerInferredReturnTypes := c.currentInferredReturnTypes

		// 2. Resolve Parameter Types
		paramTypes := []types.Type{}
		paramNames := []*parser.Identifier{}
		for _, param := range node.Parameters {
			var paramType types.Type = types.Any // Default, USE INTERFACE TYPE
			if param.TypeAnnotation != nil {
				resolvedParamType := c.resolveTypeAnnotation(param.TypeAnnotation)
				if resolvedParamType != nil {
					paramType = resolvedParamType // Assign interface{} to interface{}
				}
			}
			paramTypes = append(paramTypes, paramType)
			paramNames = append(paramNames, param.Name)
			param.ComputedType = paramType
		}

		// --- NEW: Resolve Return Type Annotation ---
		expectedReturnType := c.resolveTypeAnnotation(node.ReturnTypeAnnotation)

		// --- UPDATED: Set Context using expected type ---
		c.currentExpectedReturnType = expectedReturnType // Use resolved annotation (can be nil)
		c.currentInferredReturnTypes = nil               // Reset for this function
		if expectedReturnType == nil {
			// Only allocate if we need to infer
			c.currentInferredReturnTypes = []types.Type{}
		}

		// 4. Create function scope & define parameters
		// --- DEBUG ---
		debugPrintf("// [Checker Visit ArrowFunc] Creating Func Scope. Current Env: %p\n", c.env)
		if c.env == nil {
			panic("Checker env is nil before creating arrow scope!")
		}
		// --- END DEBUG ---
		originalEnv := c.env
		funcEnv := NewEnclosedEnvironment(originalEnv)
		c.env = funcEnv
		for i, nameNode := range paramNames {
			if !funcEnv.Define(nameNode.Value, paramTypes[i], false) {
				c.addError(nameNode, fmt.Sprintf("duplicate parameter name: %s", nameNode.Value))
			}
		}

		// 5. Visit Body
		c.visit(node.Body)
		var bodyType types.Type = types.Any
		isExprBody := false
		// Special handling for expression body
		if exprBody, ok := node.Body.(parser.Expression); ok {
			isExprBody = true
			bodyType = exprBody.GetComputedType()
			// If body is an expression, its type *is* the single inferred return type,
			// unless overridden by an annotation.
			if c.currentInferredReturnTypes != nil { // Only append if inference is active
				c.currentInferredReturnTypes = append(c.currentInferredReturnTypes, bodyType)
			}

			// --- NEW: Check expression body type against annotation ---
			if expectedReturnType != nil {
				if !c.isAssignable(bodyType, expectedReturnType) {
					// TODO: Get line number from exprBody token if possible
					c.addError(exprBody, fmt.Sprintf("cannot return expression of type '%s' from arrow function with return type annotation '%s'", bodyType.String(), expectedReturnType.String()))
				}
			}
			// --- END NEW ---
		} // Else: Body is BlockStatement, returns handled by ReturnStatement visitor

		// 6. Determine Final Return Type (Inference or Annotation)
		var finalReturnType types.Type = expectedReturnType // Start with annotation
		if finalReturnType == nil {                         // Infer ONLY if no annotation
			if len(c.currentInferredReturnTypes) == 0 {
				// If it was an expression body, we should have added its type already.
				// If it's a block body with no returns, it's Undefined.
				if isExprBody {
					// Should have been added above, but double check
					finalReturnType = bodyType
				} else {
					finalReturnType = types.Undefined // No returns in block, infer Undefined
				}
			} else {
				// Inference logic for multiple returns (existing logic seems okay)
				if len(c.currentInferredReturnTypes) == 1 {
					finalReturnType = c.currentInferredReturnTypes[0]
				} else {
					firstType := c.currentInferredReturnTypes[0]
					allSame := true
					for _, typ := range c.currentInferredReturnTypes[1:] {
						// TODO: Use proper type equality check later
						if typ != firstType { // Basic check
							allSame = false
							break
						}
					}
					if allSame {
						finalReturnType = firstType
					} else {
						// TODO: Implement Union type, fallback to Any for now
						finalReturnType = types.Any
					}
				}
			}
		} // else: Annotation exists. ReturnStatement visitor handles checks for block bodies. Expression body check done above.

		if finalReturnType == nil { // Safety check
			finalReturnType = types.Any
		}

		// 7. Create FunctionType
		funcType := &types.FunctionType{
			ParameterTypes: paramTypes,
			ReturnType:     finalReturnType,
		}

		// --- DEBUG: Log type before setting ---
		debugPrintf("// [Checker ArrowFunc] Computed funcType: %s\n", funcType.String())
		// --- END DEBUG ---

		// 8. Set ComputedType on the ArrowFunctionLiteral node
		// ... calculate funcType ...
		debugPrintf("// [Checker ArrowFunc] ABOUT TO SET Computed funcType: %#v, ReturnType: %#v\n", funcType, funcType.ReturnType)
		node.SetComputedType(funcType)

		// 9. Restore outer environment and context
		// --- DEBUG ---
		debugPrintf("// [Checker Visit ArrowFunc] Exiting Arrow Func. Restoring Env: %p (from current %p)\n", originalEnv, c.env)
		if originalEnv == nil {
			panic("Checker originalEnv is nil before restoring arrow scope!")
		}
		// --- END DEBUG ---
		c.env = originalEnv
		c.currentExpectedReturnType = outerExpectedReturnType
		c.currentInferredReturnTypes = outerInferredReturnTypes

	case *parser.CallExpression:
		// --- UPDATED: Handle CallExpression ---
		// 1. Check the expression being called
		c.visit(node.Function)
		funcNodeType := node.Function.GetComputedType()
		if funcNodeType == nil {
			// Error visiting the function expression itself
			funcIdent, isIdent := node.Function.(*parser.Identifier)
			errMsg := "cannot determine type of called expression"
			if isIdent {
				errMsg = fmt.Sprintf("cannot determine type of called identifier '%s'", funcIdent.Value)
			}
			c.addError(node, errMsg)
			node.SetComputedType(types.Any)
			return
		}

		if funcNodeType == types.Any {
			// Allow calling 'any', result is 'any'. Check args against 'any'.
			for _, argNode := range node.Arguments {
				c.visit(argNode) // Visit args even if function is 'any'
			}
			node.SetComputedType(types.Any)
			return
		}

		funcType, ok := funcNodeType.(*types.FunctionType)
		if !ok {
			c.addError(node, fmt.Sprintf("cannot call value of type '%s'", funcNodeType.String()))
			node.SetComputedType(types.Any) // Result type is unknown/error
			return
		}

		// --- MODIFIED Arity and Argument Type Checking ---
		actualArgCount := len(node.Arguments)

		if funcType.IsVariadic {
			// --- Variadic Function Check ---
			if len(funcType.ParameterTypes) == 0 {
				c.addError(node, "internal checker error: variadic function type must have at least one parameter type (for the array)")
				node.SetComputedType(types.Any) // Error state
				return
			}

			minExpectedArgs := len(funcType.ParameterTypes) - 1
			if actualArgCount < minExpectedArgs {
				c.addError(node, fmt.Sprintf("expected at least %d arguments for variadic function, but got %d", minExpectedArgs, actualArgCount))
				// Don't check args if minimum count isn't met.
			} else {
				// Check fixed arguments
				fixedArgsOk := true
				for i := 0; i < minExpectedArgs; i++ {
					argNode := node.Arguments[i]
					c.visit(argNode)
					argType := argNode.GetComputedType()
					paramType := funcType.ParameterTypes[i]
					if argType == nil { // Error visiting arg
						fixedArgsOk = false
						continue
					}
					if !c.isAssignable(argType, paramType) {
						c.addError(argNode, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'", i+1, argType.String(), paramType.String()))
						fixedArgsOk = false
					}
				}

				// Check variadic arguments
				if fixedArgsOk { // Only check variadic part if fixed part was okay
					variadicParamType := funcType.ParameterTypes[minExpectedArgs]
					arrayType, isArray := variadicParamType.(*types.ArrayType)
					if !isArray {
						c.addError(node, fmt.Sprintf("internal checker error: variadic parameter type must be an array type, got %s", variadicParamType.String()))
					} else {
						variadicElementType := arrayType.ElementType
						if variadicElementType == nil { // Should not happen with valid types
							variadicElementType = types.Any
						}
						// Check remaining arguments against the element type
						for i := minExpectedArgs; i < actualArgCount; i++ {
							argNode := node.Arguments[i]
							c.visit(argNode)
							argType := argNode.GetComputedType()
							if argType == nil { // Error visiting arg
								continue
							}
							if !c.isAssignable(argType, variadicElementType) {
								c.addError(argNode, fmt.Sprintf("variadic argument %d: cannot assign type '%s' to parameter element type '%s'", i+1, argType.String(), variadicElementType.String()))
							}
						}
					}
				}
			}
		} else {
			// --- Non-Variadic Function Check (Original Logic) ---
			expectedArgCount := len(funcType.ParameterTypes)
			if actualArgCount != expectedArgCount {
				c.addError(node, fmt.Sprintf("expected %d arguments, but got %d", expectedArgCount, actualArgCount))
				// Continue checking assignable args anyway? Let's stop if arity wrong.
			} else {
				// Check Argument Types
				for i, argNode := range node.Arguments {
					c.visit(argNode) // Visit argument to compute its type
					argType := argNode.GetComputedType()
					paramType := funcType.ParameterTypes[i]

					if argType == nil {
						// Error computing argument type, can't check assignability
						continue
					}

					if !c.isAssignable(argType, paramType) {
						c.addError(argNode, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'", i+1, argType.String(), paramType.String()))
					}
				}
			}
		}
		// --- END MODIFIED Checking ---

		// Set Result Type (unchanged)
		debugPrintf("// [Checker CallExpr] Setting result type from func '%s'. ReturnType from Sig: %T (%v)\n", node.Function.String(), funcType.ReturnType, funcType.ReturnType)
		node.SetComputedType(funcType.ReturnType)

	case *parser.AssignmentExpression:
		// Visit LHS (Identifier, IndexExpr, MemberExpr)
		c.visit(node.Left)
		lhsType := node.Left.GetComputedType()
		if lhsType == nil {
			lhsType = types.Any
		} // Handle nil from error

		// Visit RHS value
		c.visit(node.Value)
		rhsType := node.Value.GetComputedType()
		if rhsType == nil {
			rhsType = types.Any
		} // Handle nil from error

		// Widen types for operator checks
		widenedLhsType := types.GetWidenedType(lhsType) // Needed for operator checks AND assignability target
		widenedRhsType := types.GetWidenedType(rhsType)
		isAnyLhs := widenedLhsType == types.Any
		isAnyRhs := widenedRhsType == types.Any

		// Operator-Specific Pre-Checks
		validOperands := true
		switch node.Operator {
		// Arithmetic Compound Assignments (Check if LHS/RHS are numeric)
		case "+=", "-=", "*=", "/=", "%=", "**=":
			if !isAnyLhs && widenedLhsType != types.Number {
				// Exception: Allow string += any
				if !(node.Operator == "+=" && widenedLhsType == types.String) {
					c.addError(node.Left, fmt.Sprintf("operator '%s' requires LHS operand of type 'number' or 'any', got '%s'", node.Operator, widenedLhsType.String()))
					validOperands = false
				}
			}
			if !isAnyRhs && widenedRhsType != types.Number {
				// Exception: Allow string += any or number += string
				if !(node.Operator == "+=" && (widenedLhsType == types.String || widenedRhsType == types.String || isAnyRhs)) { // Adjusted check for RHS in +=
					c.addError(node.Value, fmt.Sprintf("operator '%s' requires RHS operand of type 'number', 'string' (if LHS is string), or 'any', got '%s'", node.Operator, widenedRhsType.String()))
					validOperands = false
				}
			}
			// Note: += specifically allows string concatenation, checks adjusted slightly.

		// Bitwise/Shift Compound Assignments (Require numeric operands)
		case "&=", "|=", "^=", "<<=", ">>=", ">>>=":
			if !isAnyLhs && widenedLhsType != types.Number {
				c.addError(node.Left, fmt.Sprintf("operator '%s' requires LHS operand of type 'number' or 'any', got '%s'", node.Operator, widenedLhsType.String()))
				validOperands = false
			}
			if !isAnyRhs && widenedRhsType != types.Number {
				c.addError(node.Value, fmt.Sprintf("operator '%s' requires RHS operand of type 'number' or 'any', got '%s'", node.Operator, widenedRhsType.String()))
				validOperands = false
			}

		// Logical/Coalesce Compound Assignments (No extra numeric checks needed)
		case "&&=", "||=", "??=":
			break // Handled by assignability check below

		case "=":
			// Simple assignment, no extra operator checks needed here.
			break

		default:
			c.addError(node, fmt.Sprintf("internal checker error: unhandled assignment operator %s", node.Operator))
			validOperands = false
		}

		// --- Check LHS const status ---
		// ... (keep existing const check) ...
		if identLHS, ok := node.Left.(*parser.Identifier); ok {
			_, isConst, found := c.env.Resolve(identLHS.Value)
			if found && isConst {
				c.addError(node.Left, fmt.Sprintf("cannot assign to constant variable '%s'", identLHS.Value))
				// Still proceed to check assignability for more errors
			}
		}
		// TODO: Check if MemberExpression LHS refers to a const property?

		// --- Final Assignability Check ---
		if validOperands {
			// <<< USE WIDENED LHS TYPE AS TARGET for assignability check >>>
			targetType := widenedLhsType
			// For simple identifiers, if a declared type exists, we should respect that *exact* type
			// instead of widening it for the target check.
			if identLHS, isIdent := node.Left.(*parser.Identifier); isIdent {
				resolvedType, _, found := c.env.Resolve(identLHS.Value)
				// Check if the *original* lhsType came directly from a declared type (annotation)
				// This is tricky to track perfectly. A simpler heuristic: if the original lhsType
				// isn't a literal type, maybe it came from an annotation or inference, so respect it.
				if found && resolvedType != nil {
					// If the resolved type is NOT a literal type, prefer it over the widened type.
					// This preserves stricter checking for annotated variables.
					if _, isLiteral := resolvedType.(*types.LiteralType); !isLiteral {
						targetType = resolvedType
					}
				}
			}

			if !c.isAssignable(rhsType, targetType) { // <<< Use targetType (usually widened LHS)
				// Special case for ??= handled within isAssignable now?
				// Let's keep the explicit check here for clarity just for ??=
				allowAssignment := false
				if node.Operator == "??=" && (lhsType == types.Null || lhsType == types.Undefined) {
					// Allow ??= if LHS is null/undefined, check if RHS assignable to WIDENED LHS
					if c.isAssignable(rhsType, widenedLhsType) { // Check assignability to widened target
						allowAssignment = true
					}
					// If RHS is not assignable even to widened LHS, error will be reported below
				}

				if !allowAssignment {
					leftDesc := "location"
					if ident, ok := node.Left.(*parser.Identifier); ok {
						leftDesc = fmt.Sprintf("variable '%s'", ident.Value)
					} else if _, ok := node.Left.(*parser.MemberExpression); ok {
						leftDesc = "property"
					} else if _, ok := node.Left.(*parser.IndexExpression); ok {
						leftDesc = "element"
					}
					// Report error comparing RHS to the potentially stricter targetType
					c.addError(node.Value, fmt.Sprintf("type '%s' is not assignable to %s of type '%s'", rhsType.String(), leftDesc, targetType.String()))
				}
			}
		}

		// Set computed type for the overall assignment expression (evaluates to RHS value)
		node.SetComputedType(rhsType)

	case *parser.UpdateExpression:
		// --- NEW: Handle UpdateExpression ---
		c.visit(node.Argument)
		argType := node.Argument.GetComputedType()
		resultType := types.Number // Default result is number

		if argType == nil {
			argType = types.Any // Handle nil from visit error
		}

		widenedArgType := types.GetWidenedType(argType)

		if widenedArgType != types.Number && widenedArgType != types.Any {
			// Allow 'any' for now, but error on other non-numbers
			c.addError(node.Argument, fmt.Sprintf("operator '%s' cannot be applied to type '%s'", node.Operator, widenedArgType.String()))
			resultType = types.Any // Result is Any if operand is invalid
		}

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

	// --- Loop Statements (Control flow, check condition/body) ---
	case *parser.WhileStatement:
		c.visit(node.Condition)
		c.visit(node.Body)

	case *parser.DoWhileStatement:
		c.visit(node.Body)
		c.visit(node.Condition)

	case *parser.ForStatement:
		// --- UPDATED: Handle For Statement Scope ---
		// 1. Create a new enclosed environment for the loop
		originalEnv := c.env
		loopEnv := NewEnclosedEnvironment(originalEnv)
		c.env = loopEnv
		debugPrintf("// [Checker ForStmt] Created loop scope %p (outer: %p)\n", loopEnv, originalEnv)

		// 2. Visit parts within the loop scope
		c.visit(node.Initializer)
		c.visit(node.Condition)
		c.visit(node.Update)
		c.visit(node.Body)

		// 3. Restore the outer environment
		c.env = originalEnv
		debugPrintf("// [Checker ForStmt] Restored outer scope %p (from %p)\n", originalEnv, loopEnv)

	// --- Loop Control (No specific type checking needed?) ---
	case *parser.BreakStatement:
		break // Nothing to check type-wise
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

	default:
		// Optional: Add error for unhandled node types
		// c.addError(0, fmt.Sprintf("Checker: Unhandled AST node type %T", node))
		break
	}
}

// Helper to add type errors (consider adding token/node info later)
func (c *Checker) addError(node parser.Node, message string) {
	token := GetTokenFromNode(node)
	err := &errors.TypeError{
		Position: errors.Position{
			Line:     token.Line,
			Column:   token.Column,
			StartPos: token.StartPos,
			EndPos:   token.EndPos,
		},
		Msg: message,
	}
	c.errors = append(c.errors, err)
}

// --- NEW HELPER: GetTokenFromNode (Best effort) ---

// GetTokenFromNode attempts to extract the primary token associated with a parser node.
// This is useful for getting line numbers for error reporting.
// Returns the zero value of lexer.Token if no specific token can be easily extracted.
func GetTokenFromNode(node parser.Node) lexer.Token {
	switch n := node.(type) {
	// Statements (use the primary keyword/token)
	case *parser.LetStatement:
		return n.Token
	case *parser.ConstStatement:
		return n.Token
	case *parser.ReturnStatement:
		return n.Token
	case *parser.ExpressionStatement:
		return n.Token // Token of the start of the expression
	case *parser.BlockStatement:
		return n.Token // The '{' token
	case *parser.IfExpression:
		return n.Token // The 'if' token
	case *parser.WhileStatement:
		return n.Token
	case *parser.ForStatement:
		return n.Token
	case *parser.BreakStatement:
		return n.Token
	case *parser.ContinueStatement:
		return n.Token
	case *parser.DoWhileStatement:
		return n.Token
	case *parser.TypeAliasStatement:
		return n.Token

	// Expressions (use the primary token where available)
	case *parser.Identifier:
		return n.Token
	case *parser.NumberLiteral:
		return n.Token
	case *parser.StringLiteral:
		return n.Token
	case *parser.BooleanLiteral:
		return n.Token
	case *parser.NullLiteral:
		return n.Token
	case *parser.UndefinedLiteral:
		return n.Token
	case *parser.ObjectLiteral: // <<< ADD THIS
		return n.Token // The '{' token
	case *parser.FunctionLiteral:
		return n.Token // The 'function' token
	case *parser.ArrowFunctionLiteral:
		return n.Token // The '=>' token
	case *parser.PrefixExpression:
		return n.Token // The operator token
	case *parser.InfixExpression:
		return n.Token // The operator token
	case *parser.TernaryExpression:
		return n.Token // The '?' token
	case *parser.CallExpression:
		return n.Token // The '(' token
	case *parser.IndexExpression:
		return n.Token // The '[' token
	case *parser.ArrayLiteral:
		return n.Token // The '[' token
	case *parser.AssignmentExpression:
		return n.Token // The operator token
	case *parser.UpdateExpression:
		return n.Token // The operator token
	// Add other expression types if they have a clear primary token

	// Add specific handling for UnionTypeExpression if needed, but it's primarily structural
	// case *parser.UnionTypeExpression: return n.Token // The '|' token?

	default:
		// Cannot easily determine a representative token
		return lexer.Token{} // Return zero value
	}
}

// --- NEW: Type Alias Statement Check ---

func (c *Checker) checkTypeAliasStatement(node *parser.TypeAliasStatement) {
	// Called ONLY during Pass 1 for hoisted aliases.

	// 1. Check if already defined (belt-and-suspenders check)
	// Note: We use c.env which IS the globalEnv during Pass 1
	if _, exists := c.env.ResolveType(node.Name.Value); exists {
		debugPrintf("// [Checker TypeAlias P1] Alias '%s' already defined? Skipping.\n", node.Name.Value)
		return // Should not happen if parser prevents duplicates
	}

	// 2. Resolve the RHS type using the CURRENT (global) environment
	// This allows aliases to reference previously defined aliases in the same pass
	aliasedType := c.resolveTypeAnnotation(node.Type) // Uses c.env (globalEnv)
	if aliasedType == nil {
		debugPrintf("// [Checker TypeAlias P1] Failed to resolve type for alias '%s'. Defining as Any.\n", node.Name.Value)
		if !c.env.DefineTypeAlias(node.Name.Value, types.Any) {
			debugPrintf("// [Checker TypeAlias P1] WARNING: DefineTypeAlias failed for '%s' (as Any).\n", node.Name.Value)
		}
		return
	}

	// 3. Define the alias in the CURRENT (global) environment
	if !c.env.DefineTypeAlias(node.Name.Value, aliasedType) {
		debugPrintf("// [Checker TypeAlias P1] WARNING: DefineTypeAlias failed for '%s'.\n", node.Name.Value)
	} else {
		debugPrintf("// [Checker TypeAlias P1] Defined alias '%s' as type '%s' in env %p\n", node.Name.Value, aliasedType.String(), c.env)
	}
	// No need to set computed type on the TypeAliasStatement node itself
}

// --- NEW: Array Literal Check ---

// --- ADD THIS HELPER FUNCTION ---
// (Place it somewhere before checkArrayLiteral)

// deeplyWidenObjectType creates a new ObjectType where literal property types are widened.
// Returns the original type if it's not an ObjectType.
func deeplyWidenType(t types.Type) types.Type {
	// Widen top-level literals first
	widenedT := types.GetWidenedType(t)

	// If it's an object after top-level widening, widen its properties
	if objType, ok := widenedT.(*types.ObjectType); ok {
		newFields := make(map[string]types.Type, len(objType.Properties))
		for name, propType := range objType.Properties {
			// Recursively deeply widen property types? For now, just one level.
			newFields[name] = types.GetWidenedType(propType)
		}
		return &types.ObjectType{Properties: newFields}
	}

	// If it was an array, maybe deeply widen its element type?
	if arrType, ok := widenedT.(*types.ArrayType); ok {
		// Avoid infinite recursion for recursive types: Check if elem type is same as t?
		// For now, let's not recurse into arrays here, only objects.
		// return &types.ArrayType{ElementType: deeplyWidenType(arrType.ElementType)}
		return arrType // Return array type as is for now
	}

	// Return the (potentially top-level widened) type if not an object
	return widenedT
}

// --- MODIFY checkArrayLiteral ---
func (c *Checker) checkArrayLiteral(node *parser.ArrayLiteral) {
	generalizedElementTypes := []types.Type{} // Store generalized types
	for _, elemNode := range node.Elements {
		c.visit(elemNode) // Visit element to compute its type
		elemType := elemNode.GetComputedType()
		if elemType == nil {
			elemType = types.Any
		} // Handle error

		// --- Use deeplyWidenType on each element ---
		generalizedType := deeplyWidenType(elemType)
		// --- End Deep Widen ---

		generalizedElementTypes = append(generalizedElementTypes, generalizedType)
	}

	// Determine the element type for the array using the GENERALIZED types.
	var finalElementType types.Type
	if len(generalizedElementTypes) == 0 {
		finalElementType = types.Unknown
	} else {
		// Use NewUnionType to flatten/uniquify GENERALIZED element types
		finalElementType = types.NewUnionType(generalizedElementTypes...)
		// NewUnionType should simplify if all generalized types are identical
	}

	// Create the ArrayType
	arrayType := &types.ArrayType{ElementType: finalElementType}

	// Set the computed type for the ArrayLiteral node itself
	node.SetComputedType(arrayType)

	debugPrintf("// [Checker ArrayLit] Computed ElementType: %s, Full ArrayType: %s\n", finalElementType.String(), arrayType.String())
}

// --- NEW: Index Expression Check ---
func (c *Checker) checkIndexExpression(node *parser.IndexExpression) {
	// 1. Visit the base expression (array/object)
	c.visit(node.Left)
	leftType := node.Left.GetComputedType()

	// 2. Visit the index expression
	c.visit(node.Index)
	indexType := node.Index.GetComputedType()

	var resultType types.Type = types.Any // Default result type on error

	// 3. Check base type (allow Array for now)
	switch base := leftType.(type) {
	case *types.ArrayType:
		// Base is ArrayType
		// 4. Check index type (must be number for array)
		if !c.isAssignable(indexType, types.Number) {
			c.addError(node.Index, fmt.Sprintf("array index must be of type number, got %s", indexType.String()))
			// Proceed with Any as result type
		} else {
			// Index type is valid, result type is the array's element type
			if base.ElementType != nil {
				resultType = base.ElementType
			} else {
				resultType = types.Unknown
			}
		}

	case *types.ObjectType: // <<< NEW CASE
		// Base is ObjectType
		// Index must be string or number (or any)
		isIndexStringLiteral := false
		var indexStringValue string
		if litIndex, ok := indexType.(*types.LiteralType); ok && litIndex.Value.Type == vm.TypeString {
			isIndexStringLiteral = true
			indexStringValue = vm.AsString(litIndex.Value)
		}

		widenedIndexType := types.GetWidenedType(indexType)

		if widenedIndexType == types.String || widenedIndexType == types.Number || widenedIndexType == types.Any {
			if isIndexStringLiteral {
				// Index is a specific string literal, look it up directly
				propType, exists := base.Properties[indexStringValue]
				if exists {
					if propType == nil { // Safety check
						resultType = types.Never
						c.addError(node.Index, fmt.Sprintf("internal checker error: property '%s' has nil type", indexStringValue))
					} else {
						resultType = propType
					}
				} else {
					c.addError(node.Index, fmt.Sprintf("property '%s' does not exist on type %s", indexStringValue, base.String()))
					// resultType remains Error
				}
			} else {
				// Index is a general string/number/any - cannot determine specific property type.
				// TODO: Support index signatures later?
				// For now, result is 'any' as we don't know which property is accessed.
				resultType = types.Any
			}
		} else {
			// Invalid index type for object
			c.addError(node.Index, fmt.Sprintf("object index must be of type 'string', 'number', or 'any', got '%s'", indexType.String()))
			// resultType remains Error
		}

	case *types.Primitive:
		// Allow indexing on strings?
		if base == types.String {
			// 4. Check index type (must be number for string)
			if !c.isAssignable(indexType, types.Number) {
				c.addError(node.Index, fmt.Sprintf("string index must be of type number, got %s", indexType.String()))
			}
			// Result of indexing a string is always a string (or potentially undefined)
			// Let's use string | undefined? Or just string?
			// For simplicity now, let's say string.
			// A more precise type might be: types.NewUnionType(types.String, types.Undefined)
			resultType = types.String
		} else {
			c.addError(node.Index, fmt.Sprintf("cannot apply index operator to type %s", leftType.String()))
		}

	default:
		c.addError(node.Index, fmt.Sprintf("cannot apply index operator to type %s", leftType.String()))
	}

	// Set computed type on the IndexExpression node
	node.SetComputedType(resultType)
	debugPrintf("// [Checker IndexExpr] Computed type: %s\n", resultType.String())
}

// Helper function
func (c *Checker) checkMemberExpression(node *parser.MemberExpression) {
	// 1. Visit the object part
	c.visit(node.Object)
	objectType := node.Object.GetComputedType()
	if objectType == nil {
		// If visiting the object failed, checker should have added error.
		// Set objectType to Any to prevent cascading nil errors here.
		objectType = types.Any
	}

	// 2. Get the property name (Property is always an Identifier in MemberExpression)
	propertyName := node.Property.Value // node.Property is *parser.Identifier

	// 3. Widen the object type for checks
	widenedObjectType := types.GetWidenedType(objectType)

	var resultType types.Type = types.Never // Default to Never if property not found/invalid access

	// 4. Handle different base types
	if widenedObjectType == types.Any {
		resultType = types.Any // Property access on 'any' results in 'any'
	} else if widenedObjectType == types.String {
		if propertyName == "length" {
			resultType = types.Number // string.length is number
		} else {
			c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type 'string'", propertyName))
			// resultType remains types.Error
		}
	} else {
		// Use a type switch for struct-based types
		switch obj := widenedObjectType.(type) {
		case *types.ArrayType:
			if propertyName == "length" {
				resultType = types.Number // Array.length is number
			} else {
				// TODO: Add array methods later? (e.g., .push)
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
				// resultType remains types.Error
			}
		case *types.ObjectType: // <<< MODIFIED CASE
			// Look for the property in the object's fields
			fieldType, exists := obj.Properties[propertyName]
			if exists {
				// Property found
				if fieldType == nil { // Should ideally not happen if checker populates correctly
					c.addError(node.Property, fmt.Sprintf("internal checker error: property '%s' has nil type in ObjectType", propertyName))
					resultType = types.Never
				} else {
					resultType = fieldType
				}
			} else {
				// Property not found
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
				// resultType remains types.Error
			}
		// Add cases for other struct-based types here if needed (e.g., FunctionType methods?)
		default:
			// This covers cases where widenedObjectType was not String, Any, ArrayType, ObjectType, etc.
			// e.g., trying to access property on number, boolean, null, undefined
			c.addError(node.Object, fmt.Sprintf("property access is not supported on type %s", widenedObjectType.String()))
			// resultType remains types.Error
		}
	}

	// 5. Set the computed type on the MemberExpression node itself
	node.SetComputedType(resultType)
	debugPrintf("// [Checker MemberExpr] ObjectType: %s, Property: %s, ResultType: %s\n", objectType.String(), propertyName, resultType.String())
}

// --- NEW: Switch Statement Check ---

func (c *Checker) checkSwitchStatement(node *parser.SwitchStatement) {
	// 1. Visit the switch expression
	c.visit(node.Expression)
	switchExprType := node.Expression.GetComputedType()
	if switchExprType == nil {
		switchExprType = types.Any // Default to Any if expression check failed
	}
	widenedSwitchExprType := types.GetWidenedType(switchExprType)

	// 2. Visit cases
	for _, caseClause := range node.Cases {
		if caseClause.Condition != nil {
			// Visit case condition
			c.visit(caseClause.Condition)
			caseCondType := caseClause.Condition.GetComputedType()
			if caseCondType == nil {
				caseCondType = types.Any // Default to Any on error
			}
			widenedCaseCondType := types.GetWidenedType(caseCondType)

			// --- Basic Comparability Check --- (Simplified for now)
			// Allow comparison if either is Any/Unknown or if they are the same primitive type.
			// This is not perfect for === semantics but catches basic errors.
			isAny := widenedSwitchExprType == types.Any || widenedCaseCondType == types.Any
			isUnknown := widenedSwitchExprType == types.Unknown || widenedCaseCondType == types.Unknown
			_, isSwitchFunc := widenedSwitchExprType.(*types.FunctionType)
			_, isCaseFunc := widenedCaseCondType.(*types.FunctionType)
			_, isSwitchArray := widenedSwitchExprType.(*types.ArrayType)
			_, isCaseArray := widenedCaseCondType.(*types.ArrayType)

			incompatible := false
			if (isSwitchFunc != isCaseFunc) && !isAny && !isUnknown {
				incompatible = true // Can't compare func and non-func (unless any/unknown)
			}
			if (isSwitchArray != isCaseArray) && !isAny && !isUnknown {
				// Maybe allow comparing array to any/null/undefined? Needs refinement.
				// For now, treat array vs non-array as incompatible unless any/unknown involved.
				incompatible = true
			}
			// Add more checks? e.g., number vs string? Current VM might coerce.
			// Strict equality usually doesn't coerce, so maybe check primitives match?

			if incompatible {
				c.addError(caseClause.Condition, fmt.Sprintf("this case expression type (%s) is not comparable to the switch expression type (%s)", widenedCaseCondType, widenedSwitchExprType))
			}
			// --- End Comparability Check ---
		}

		// Visit case body (BlockStatement, handles its own scope)
		c.visit(caseClause.Body)
	}

	// Switch statements don't produce a value themselves
	// node.SetComputedType(types.Void) // Remove this line
}

// ... (after checkIndexExpression) ...

// checkObjectLiteral checks the type of an object literal expression.
func (c *Checker) checkObjectLiteral(node *parser.ObjectLiteral) {
	fields := make(map[string]types.Type)
	seenKeys := make(map[string]bool)

	for _, prop := range node.Properties {
		var keyName string
		switch key := prop.Key.(type) {
		case *parser.Identifier:
			keyName = key.Value
		case *parser.StringLiteral:
			keyName = key.Value
		case *parser.NumberLiteral: // Allow number literals as keys, convert to string
			// Note: JavaScript converts number keys to strings internally
			keyName = fmt.Sprintf("%v", key.Value) // Simple conversion
		default:
			c.addError(prop.Key, "object key must be an identifier, string, or number literal")
			continue // Skip this property if key type is invalid
		}

		// Check for duplicate keys
		if seenKeys[keyName] {
			c.addError(prop.Key, fmt.Sprintf("duplicate property key: '%s'", keyName))
			// Continue checking value type anyway? Yes, to catch more errors.
		}
		seenKeys[keyName] = true

		// Visit the value to determine its type
		c.visit(prop.Value)
		valueType := prop.Value.GetComputedType()
		if valueType == nil {
			// If visiting the value failed, checker should have added error.
			// Default to Any to prevent cascading nil errors.
			valueType = types.Any
		}

		// Store the resolved type (use widened type for consistency?)
		// Let's use the direct type for now, widening happens on access/assignment.
		fields[keyName] = valueType
		// Set type on property node itself? Optional, maybe not needed.
		// prop.ComputedType = valueType
	}

	// Create the ObjectType
	objType := &types.ObjectType{Properties: fields}

	// Set the computed type for the ObjectLiteral node itself
	node.SetComputedType(objType)
	debugPrintf("// [Checker ObjectLit] Computed type: %s\n", objType.String())
}

// --- NEW: Helper to resolve FunctionLiteral signature to types.FunctionType ---
// Resolves parameter and return type annotations within the given environment.
func (c *Checker) resolveFunctionLiteralType(node *parser.FunctionLiteral, env *Environment) *types.FunctionType {
	paramTypes := []types.Type{}
	for _, paramNode := range node.Parameters {
		var resolvedParamType types.Type
		if paramNode.TypeAnnotation != nil {
			// Temporarily use the provided environment for resolving the annotation
			originalEnv := c.env
			c.env = env
			resolvedParamType = c.resolveTypeAnnotation(paramNode.TypeAnnotation)
			c.env = originalEnv // Restore original environment
		}

		if resolvedParamType == nil {
			resolvedParamType = types.Any // Default to Any if no annotation or resolution failed
		}
		paramTypes = append(paramTypes, resolvedParamType)
	}

	var resolvedReturnType types.Type         // Keep as interface type
	var resolvedTypeFromAnnotation types.Type // Temp variable, also interface type

	funcNameForLog := "<anonymous_resolve>"
	if node.Name != nil {
		funcNameForLog = node.Name.Value
	}

	if node.ReturnTypeAnnotation != nil {
		originalEnv := c.env
		c.env = env
		debugPrintf("// [Checker resolveFuncLitType] Resolving return annotation (%T) for func '%s'\n", node.ReturnTypeAnnotation, funcNameForLog)
		// Assign to the temporary variable first
		resolvedTypeFromAnnotation = c.resolveTypeAnnotation(node.ReturnTypeAnnotation)
		debugPrintf("// [Checker resolveFuncLitType] resolveTypeAnnotation returned: Ptr=%p, Type=%T, Value=%#v for func '%s'\n", resolvedTypeFromAnnotation, resolvedTypeFromAnnotation, resolvedTypeFromAnnotation, funcNameForLog)
		// DO NOT assign to resolvedReturnType here yet
		c.env = originalEnv
	} else {
		debugPrintf("// [Checker resolveFuncLitType] No return type annotation for func '%s'\n", funcNameForLog)
		resolvedTypeFromAnnotation = nil // Ensure temp var is nil if no annotation
	}

	// Now, assign the result from the temporary variable to the main one
	resolvedReturnType = resolvedTypeFromAnnotation
	debugPrintf("// [Checker resolveFuncLitType] Assigned final resolvedReturnType: Ptr=%p, Type=%T, Value=%#v for func '%s'\n", resolvedReturnType, resolvedReturnType, resolvedReturnType, funcNameForLog) // ADDED LOG

	// The final check should now behave correctly
	if resolvedReturnType == nil {
		debugPrintf("// [Checker resolveFuncLitType] Final check: resolvedReturnType is nil for func '%s'\n", funcNameForLog)
		// resolvedReturnType = nil // It's already nil, no need to re-assign
	} else {
		debugPrintf("// [Checker resolveFuncLitType] Final check: resolvedReturnType is NOT nil (Ptr=%p) for func '%s'\n", resolvedReturnType, funcNameForLog)
	}

	return &types.FunctionType{
		ParameterTypes: paramTypes,
		ReturnType:     resolvedReturnType, // Use the value assigned outside the if
	}
}

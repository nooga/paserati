package checker

import (
	"fmt"
	"paserati/pkg/errors" // Added import
	"paserati/pkg/source" // Added import for source context

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
		// Add more type cases as needed
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

// ContextualType represents type information that flows from context to sub-expressions
type ContextualType struct {
	ExpectedType types.Type // The type expected in this context
	IsContextual bool       // Whether this is a contextual hint vs. required type
}

// Checker performs static type checking on the AST.
type Checker struct {
	program *parser.Program // Root AST node
	source  *source.SourceFile // Source context for error reporting (cached from program)
	// TODO: Add Type Registry if needed
	env    *Environment           // Current type environment
	errors []errors.PaseratiError // Changed from []TypeError

	// --- NEW: Context for checking function bodies ---
	// Expected return type of the function currently being checked (set by explicit annotation).
	currentExpectedReturnType types.Type
	// List of types found in return statements within the current function (used for inference).
	currentInferredReturnTypes []types.Type

	// --- NEW: Context for 'this' type checking ---
	// Type of 'this' in the current context (set when checking methods)
	currentThisType types.Type
}

// NewChecker creates a new type checker.
func NewChecker() *Checker {
	return &Checker{
		env:    NewGlobalEnvironment(),   // Create persistent global environment
		errors: []errors.PaseratiError{}, // Initialize with correct type
		// Initialize function context fields to nil/empty
		currentExpectedReturnType:  nil,
		currentInferredReturnTypes: nil,
		currentThisType:            nil, // Initialize this type context
	}
}

// Check analyzes the given program AST for type errors.
func (c *Checker) Check(program *parser.Program) []errors.PaseratiError {
	c.program = program
	c.source = program.Source // Cache source for error reporting
	c.errors = []errors.PaseratiError{} // Reset errors
	// DON'T reset the environment - keep it persistent for REPL sessions
	// c.env = NewGlobalEnvironment()      // Start with a fresh global environment for this check
	globalEnv := c.env

	// --- Data Structures for Passes ---
	nodesProcessedPass1 := make(map[parser.Node]bool)   // Nodes handled in Pass 1 (Type Aliases)
	nodesProcessedPass2 := make(map[parser.Node]bool)   // Nodes handled in Pass 2 (Signatures/Vars)
	functionsToVisitBody := []*parser.FunctionLiteral{} // Function literals needing body check in Pass 3

	// --- Pass 1: Define ALL Type Aliases ---
	debugPrintf("\n// --- Checker - Pass 1: Defining Type Aliases and Interfaces ---\n")
	for _, stmt := range program.Statements {
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
				FunctionName:              name,
				TypeParameters:            funcLit.TypeParameters, // Support for generic functions
				Parameters:                funcLit.Parameters,
				RestParameter:             funcLit.RestParameter,
				ReturnTypeAnnotation:      funcLit.ReturnTypeAnnotation,
				Body:                      nil, // Don't process body during hoisting
				IsArrow:                   false,
				AllowSelfReference:        false, // Don't allow self-reference during hoisting
				AllowOverloadCompletion:   false, // Don't check overloads during hoisting
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
		outerThisType := c.currentThisType
		c.currentExpectedReturnType = funcSignature.ReturnType // Use return type from initial signature
		c.currentInferredReturnTypes = nil
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
						Index:      i,
					}
					debugPrintf("// [Checker Pass 3] WARNING: Had to create new type parameter '%s'\n", typeParam.Name)
				}
				
				// Define it in the environment
				if !typeParamEnv.DefineTypeParameter(typeParam.Name, typeParam) {
					c.addError(typeParamNode.Name, fmt.Sprintf("duplicate type parameter name: %s", typeParam.Name))
				}
				
				// Set computed type on the AST node
				typeParamNode.SetComputedType(&types.TypeParameterType{Parameter: typeParam})
				
				debugPrintf("// [Checker Pass 3] Defined type parameter '%s' for body checking (reused from hoisting)\n", typeParam.Name)
			}
		}
		
		funcEnv := NewEnclosedEnvironment(typeParamEnv)
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
			if !funcEnv.Define(funcLit.RestParameter.Name.Value, funcSignature.RestParameterType, false) {
				c.addError(funcLit.RestParameter.Name, fmt.Sprintf("duplicate parameter name: %s", funcLit.RestParameter.Name.Value))
			}
			// Set computed type on the RestParameter node itself
			funcLit.RestParameter.ComputedType = funcSignature.RestParameterType
			debugPrintf("// [Checker Pass 3] Defined rest parameter '%s' with type: %s\n", funcLit.RestParameter.Name.Value, funcSignature.RestParameterType.String())
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
				actualReturnType = types.Undefined
			} else {
				actualReturnType = types.NewUnionType(c.currentInferredReturnTypes...)
			}
			debugPrintf("// [Checker Pass 3] Inferred return type for '%s': %s\n", funcNameForLog, actualReturnType.String())
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
		c.currentThisType = outerThisType
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

				// Get the variable's type defined in Pass 2 to check if we have a type annotation
				variableType, _, found := globalEnv.Resolve(varName.Value)
				if !found { // Should not happen
					debugPrintf("// [Checker Pass 4] ERROR: Variable '%s' not found in env during final check?\n", varName.Value)
					continue
				}

				// Use contextual typing if we have a type annotation (not Any)
				if typeAnnotation != nil && variableType != types.Any {
					debugPrintf("// [Checker Pass 4] Using contextual typing for '%s' with expected type: %s\n", varName.Value, variableType.String())
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
				assignable := types.IsAssignable(computedInitializerType, variableType)

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
						finalInferredType = types.DeeplyWidenType(computedInitializerType) // Use the deep widen helper
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
			c.visit(node.Expression)

			// Special handling for FunctionSignature expressions
			if sigExpr, ok := node.Expression.(*parser.FunctionSignature); ok {
				c.processFunctionSignature(sigExpr)
			}

		// TODO: Handle other top-level statement types if necessary
		default:
			debugPrintf("// [Checker Pass 4] Visiting unhandled statement type %T\n", node)
			c.visit(node) // Fallback visit? Might be unnecessary
		}
	}
	debugPrintf("// --- Checker - Pass 4: Complete ---\n")

	return c.errors
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
			// Create unified ObjectType for preliminary function type
			prelimSig := &types.Signature{
				ParameterTypes: prelimParamTypes,
				ReturnType:     prelimReturnType,
			}
			preliminaryInitializerType = types.NewFunctionType(prelimSig)
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
			// Use contextual typing if we have a declared type
			if declaredType != nil {
				c.visitWithContext(node.Value, &ContextualType{
					ExpectedType: declaredType,
					IsContextual: true,
				})
			} else {
				c.visit(node.Value) // Regular visit if no type annotation
			}

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
				assignable := types.IsAssignable(computedInitializerType, declaredType)

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
			// Use contextual typing if we have a declared type
			if declaredType != nil {
				c.visitWithContext(node.Value, &ContextualType{
					ExpectedType: declaredType,
					IsContextual: true,
				})
			} else {
				c.visit(node.Value) // Regular visit if no type annotation
			}
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
			assignable := types.IsAssignable(computedInitializerType, declaredType)

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

	case *parser.ArrayDestructuringDeclaration:
		c.checkArrayDestructuringDeclaration(node)
		
	case *parser.ObjectDestructuringDeclaration:
		c.checkObjectDestructuringDeclaration(node)

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
			
			if !types.IsAssignable(actualReturnType, c.currentExpectedReturnType) {
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

	case *parser.StringLiteral:
		literalType := &types.LiteralType{Value: vm.String(node.Value)}
		node.SetComputedType(literalType) // <<< USE NODE METHOD

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
			// Check for built-in function before reporting error
			builtinType := c.getBuiltinType(node.Value)
			if builtinType != nil {
				debugPrintf("// [Checker Debug] visit(Identifier): '%s' resolved as built-in type: %s\n", node.Value, builtinType.String())
				node.SetComputedType(builtinType)
				node.IsConstant = true // Built-ins are effectively constants
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
				// The operand must be a property access expression
				switch node.Right.(type) {
				case *parser.MemberExpression, *parser.IndexExpression:
					// Valid delete target
				case *parser.Identifier:
					// In TypeScript, deleting a variable is not allowed
					c.addError(node, "delete cannot be applied to variables")
				default:
					// Other expressions are not valid delete targets
					c.addError(node, fmt.Sprintf("delete cannot be applied to %T", node.Right))
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

		// 2. Detect type guards in the condition
		typeGuard := c.detectTypeGuard(node.Condition)

		// 3. Check Consequence block (potentially with narrowed environment)
		originalEnv := c.env
		narrowedEnv := c.applyTypeNarrowing(typeGuard)

		if narrowedEnv != nil {
			debugPrintf("// [Checker IfExpr] Applying type narrowing in consequence block\n")
			c.env = narrowedEnv // Use narrowed environment for consequence
		}

		c.visit(node.Consequence)

		// Restore original environment before checking alternative
		c.env = originalEnv

		// 4. Check Alternative block (if it exists) - potentially with inverted type narrowing
		if node.Alternative != nil {
			// Apply inverted type narrowing for else branch
			invertedEnv := c.applyInvertedTypeNarrowing(typeGuard)

			if invertedEnv != nil {
				debugPrintf("// [Checker IfExpr] Applying inverted type narrowing in alternative block\n")
				c.env = invertedEnv // Use inverted narrowed environment for alternative
			}

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

		// 2. Detect type guards in the condition
		typeGuard := c.detectTypeGuard(node.Condition)

		// 3. Check Consequence block (potentially with narrowed environment)
		originalEnv := c.env
		narrowedEnv := c.applyTypeNarrowing(typeGuard)

		if narrowedEnv != nil {
			debugPrintf("// [Checker IfStmt] Applying type narrowing in consequence block\n")
			c.env = narrowedEnv // Use narrowed environment for consequence
		}

		c.visit(node.Consequence)

		// Restore original environment before checking alternative
		c.env = originalEnv

		// 4. Check Alternative block (if it exists) - potentially with inverted type narrowing
		if node.Alternative != nil {
			// Apply inverted type narrowing for else branch
			invertedEnv := c.applyInvertedTypeNarrowing(typeGuard)

			if invertedEnv != nil {
				debugPrintf("// [Checker IfStmt] Applying inverted type narrowing in alternative block\n")
				c.env = invertedEnv // Use inverted narrowed environment for alternative
			}

			c.visit(node.Alternative)

			// Restore original environment after alternative
			c.env = originalEnv
		}

		// 5. IfStatement doesn't have a value/type (it's a statement, not expression)

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

	case *parser.CallExpression:
		c.checkCallExpression(node)

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

	// --- NEW: Optional Chaining Expression Type Checking ---
	case *parser.OptionalChainingExpression:
		c.checkOptionalChainingExpression(node)

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
		funcEnv := NewEnclosedEnvironment(originalEnv)
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
				actualReturnType = types.Undefined
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

		// Spread can only be applied to array-like types
		// For now, we'll accept arrays and 'any' type
		if argType != types.Any {
			if _, isArray := argType.(*types.ArrayType); !isArray {
				c.addError(node, fmt.Sprintf("spread syntax can only be applied to arrays, got '%s'", argType.String()))
			}
		}

		// The type of a spread element is the element type of the array (for type checking purposes)
		// But this depends on context - in call expressions, it's handled specially
		// For now, set it to the original type
		node.SetComputedType(argType)

	// --- Exception Handling Statements ---
	case *parser.TryStatement:
		c.checkTryStatement(node)

	case *parser.ThrowStatement:
		c.checkThrowStatement(node)

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
	} else {
		// Not an array-like type
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
			// For now, we'll use the element type if it's not undefined
			if elemType == types.Undefined {
				elemType = types.GetWidenedType(defaultType)
			}
		}

		// Define the variable(s) with inferred type - support both identifiers and nested patterns
		c.checkDestructuringTargetForDeclaration(element.Target, elemType, node.IsConst)
	}
}

// checkObjectDestructuringDeclaration handles type checking for object destructuring declarations
func (c *Checker) checkObjectDestructuringDeclaration(node *parser.ObjectDestructuringDeclaration) {
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

	// Check if the value is an object-like type
	var objType *types.ObjectType
	if ot, ok := valueType.(*types.ObjectType); ok {
		objType = ot
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
		if objType != nil {
			if pt, exists := objType.Properties[prop.Key.Value]; exists {
				propType = pt
			}
		} else if valueType == types.Any {
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
			// For now, we'll use the property type if it's not undefined
			if propType == types.Undefined {
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
				if prop.Key != nil {
					extractedProps[prop.Key.Value] = struct{}{}
				}
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
		// TODO: Add contextual typing for object literals
		c.visit(node)
	default:
		// For other node types, use regular visit for now
		c.visit(node)
	}
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
		if !c.env.Define(clause.Parameter.Value, types.Any, false) {
			c.addError(clause.Parameter, fmt.Sprintf("parameter '%s' already declared", clause.Parameter.Value))
		}
		clause.Parameter.SetComputedType(types.Any)
	}
	
	// Check the catch body
	c.visit(clause.Body)
	
	// Restore the original environment
	c.env = originalEnv
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

package checker

import (
	"fmt"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// TypeError represents an error found during type checking.
type TypeError struct {
	Line    int    // Line number where the error occurred
	Message string // Description of the error
}

func (e *TypeError) Error() string {
	return fmt.Sprintf("Type Error (Line %d): %s", e.Line, e.Message)
}

// Environment manages type information within scopes.
type Environment struct {
	symbols     map[string]types.Type // Stores type bindings for variables/constants
	typeAliases map[string]types.Type // Stores resolved types for type aliases
	outer       *Environment          // Pointer to the enclosing environment
}

// NewEnvironment creates a new top-level type environment.
func NewEnvironment() *Environment {
	return &Environment{
		symbols:     make(map[string]types.Type),
		typeAliases: make(map[string]types.Type), // Initialize
		outer:       nil,
	}
}

// NewEnclosedEnvironment creates a new environment nested within an outer one.
func NewEnclosedEnvironment(outer *Environment) *Environment {
	return &Environment{
		symbols:     make(map[string]types.Type),
		typeAliases: make(map[string]types.Type), // Initialize
		outer:       outer,
	}
}

// Define adds a new *variable* type binding to the current environment scope.
func (e *Environment) Define(name string, typ types.Type) bool {
	e.symbols[name] = typ
	return true
}

// DefineTypeAlias adds a new *type alias* binding to the current environment scope.
// Returns false if the alias name conflicts with an existing variable OR type alias in this scope.
func (e *Environment) DefineTypeAlias(name string, typ types.Type) bool {
	// Check for conflict with existing variable/constant in this scope
	if _, exists := e.symbols[name]; exists {
		return false
	}
	// Check for conflict with existing type alias in this scope
	if _, exists := e.typeAliases[name]; exists {
		return false
	}
	e.typeAliases[name] = typ
	return true
}

// Resolve looks up a *variable* name in the current environment and its outer scopes.
func (e *Environment) Resolve(name string) (types.Type, bool) {
	// --- DEBUG ---
	fmt.Printf("// [Env Resolve] env=%p, name='%s', outer=%p\n", e, name, e.outer) // Log entry
	if e == nil {
		fmt.Printf("// [Env Resolve] ERROR: Attempted to resolve '%s' on nil environment!\n", name)
		// Prevent panic, but this indicates a bug elsewhere.
		return nil, false
	}
	if e.symbols == nil {
		fmt.Printf("// [Env Resolve] ERROR: env %p has nil symbols map!\n", e)
		// Prevent panic, indicate bug.
		return nil, false
	}
	// --- END DEBUG ---

	// Check current scope first
	typ, ok := e.symbols[name]
	if ok {
		fmt.Printf("// [Env Resolve] Found '%s' in env %p\n", name, e) // DEBUG
		return typ, true
	}

	// If not found and there's an outer scope, check there recursively
	if e.outer != nil {
		fmt.Printf("// [Env Resolve] '%s' not in env %p, checking outer %p...\n", name, e, e.outer) // DEBUG
		return e.outer.Resolve(name)
	}

	// Not found in any scope
	fmt.Printf("// [Env Resolve] '%s' not found in env %p (no outer)\n", name, e) // DEBUG
	return nil, false
}

// ResolveType looks up a *type name* (could be alias or primitive) in the current environment and its outer scopes.
// Returns the resolved type and true if found, otherwise nil and false.
func (e *Environment) ResolveType(name string) (types.Type, bool) {
	// --- DEBUG ---
	fmt.Printf("// [Env ResolveType] env=%p, name='%s', outer=%p\n", e, name, e.outer)
	if e == nil {
		return nil, false
	} // Safety
	if e.typeAliases == nil {
		fmt.Printf("// [Env ResolveType] ERROR: env %p has nil typeAliases map!\n", e)
		return nil, false
	}
	// --- END DEBUG ---

	// 1. Check type aliases in current scope
	typ, ok := e.typeAliases[name]
	if ok {
		fmt.Printf("// [Env ResolveType] Found alias '%s' in env %p\n", name, e)
		return typ, true
	}

	// 2. If not found in current aliases, check outer scopes recursively
	if e.outer != nil {
		fmt.Printf("// [Env ResolveType] Alias '%s' not in env %p, checking outer %p...\n", name, e, e.outer)
		return e.outer.ResolveType(name)
	}

	// 3. If not found in any alias scope, check built-in primitives (only at global level?)
	//    (This check is actually done in the Checker's resolveTypeAnnotation after trying env.ResolveType)

	fmt.Printf("// [Env ResolveType] Alias '%s' not found in env %p (no outer)\n", name, e)
	return nil, false
}

// Checker performs static type checking on the AST.
type Checker struct {
	program *parser.Program // Root AST node
	// TODO: Add Type Registry if needed
	env    *Environment // Current type environment
	errors []TypeError

	// --- NEW: Context for checking function bodies ---
	// Expected return type of the function currently being checked (set by explicit annotation).
	currentExpectedReturnType types.Type
	// List of types found in return statements within the current function (used for inference).
	currentInferredReturnTypes []types.Type
	computedTypes              map[parser.Node]types.Type // Store computed types for nodes
}

// NewChecker creates a new type checker.
func NewChecker() *Checker {
	return &Checker{
		env:    NewEnvironment(), // Start with a global environment
		errors: []TypeError{},
		// Initialize function context fields to nil/empty
		currentExpectedReturnType:  nil,
		currentInferredReturnTypes: nil,
		computedTypes:              make(map[parser.Node]types.Type), // Initialize the map
	}
}

// Check analyzes the given program AST for type errors.
func (c *Checker) Check(program *parser.Program) []TypeError {
	c.program = program
	c.errors = []TypeError{} // Reset errors

	// Start traversal from the program root
	c.visit(program)

	return c.errors
}

// --- Helper Functions ---

// resolveTypeAnnotation converts a parser node representing a type annotation
// into a types.Type representation.
// TODO: Expand to handle complex type expressions (arrays, functions, objects etc.)
func (c *Checker) resolveTypeAnnotation(node parser.Expression) types.Type {
	if node == nil {
		// No annotation provided, perhaps default to any or handle elsewhere?
		// For now, returning nil might be okay, caller decides default.
		return nil
	}

	// Dispatch based on the structure of the type expression node
	switch node := node.(type) {
	case *parser.Identifier:
		// --- UPDATED: Prioritize alias resolution ---
		// 1. Attempt to resolve as a type alias in the environment
		resolvedAlias, found := c.env.ResolveType(node.Value)
		if found {
			return resolvedAlias // Successfully resolved as an alias
		}

		// 2. If not found as an alias, check against known primitive type names
		switch node.Value {
		case "number":
			return types.Number
		case "string":
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
		case "void": // Added Void type check
			return types.Void
		default:
			// 3. If neither alias nor primitive, it's an unknown type name
			line := node.Token.Line
			c.addError(line, fmt.Sprintf("unknown type name: %s", node.Value))
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

	// --- NEW: Handle Literal Type Nodes ---
	case *parser.StringLiteral:
		return &types.LiteralType{Value: vm.String(node.Value)}
	case *parser.NumberLiteral:
		return &types.LiteralType{Value: vm.Number(node.Value)}
	case *parser.BooleanLiteral:
		return &types.LiteralType{Value: vm.Bool(node.Value)}
	// --- End Literal Type Nodes ---

	// TODO: Add cases for other complex type expressions (Array, Function, Object)
	// case *parser.ArrayTypeExpression: ...
	// case *parser.FunctionTypeExpression: ...

	default:
		// If we get here, the parser created a node type that resolveTypeAnnotation doesn't handle yet.
		line := 0 // TODO: Get line number from node token
		c.addError(line, fmt.Sprintf("unsupported type annotation node: %T", node))
		return nil // Indicate error
	}
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
	// Handle nil nodes gracefully (e.g., from parsing errors)
	if node == nil {
		return
	}
	// --- DEBUG: Log entry and current env ---
	fmt.Printf("// [Checker Visit Enter] Node: %T, Env: %p\n", node, c.env)
	// --- END DEBUG ---

	// Dispatch based on node type
	switch node := node.(type) {
	// --- Program ---
	case *parser.Program:
		for _, stmt := range node.Statements {
			c.visit(stmt)
		}

	// --- Statements ---
	case *parser.TypeAliasStatement:
		c.checkTypeAliasStatement(node)

	case *parser.ExpressionStatement:
		c.visit(node.Expression)
		// TODO: Check if expression result is used? (e.g., void context)

	case *parser.LetStatement:
		// --- UPDATED: Handle LetStatement ---
		// 1. Handle Type Annotation (if present)
		var declaredType types.Type
		if node.TypeAnnotation != nil {
			declaredType = c.resolveTypeAnnotation(node.TypeAnnotation)
		} else {
			declaredType = nil // Indicates type inference is needed
		}

		// --- FIX V2 for recursive functions assigned to variables ---
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
		if !c.env.Define(node.Name.Value, tempType) {
			// If Define fails here, it's a true redeclaration error.
			c.addError(node.Name.Token.Line, fmt.Sprintf("variable '%s' already declared in this scope", node.Name.Value))
		}
		fmt.Printf("// [Checker LetStmt] Temp Define '%s' as: %s\n", node.Name.Value, tempType.String()) // DEBUG
		// --- END FIX V2 ---

		// 2. Handle Initializer (if present)
		var finalInitializerType types.Type // Renamed for clarity
		if node.Value != nil {
			c.visit(node.Value) // Compute type of the initializer
			finalInitializerType = c.GetComputedTypeOrAny(node.Value)
		} else {
			finalInitializerType = nil // No initializer
		}

		// 3. Determine the final type and check assignability
		finalVariableType := declaredType // Renamed for clarity
		if finalVariableType == nil {     // Infer from initializer or lack thereof
			if finalInitializerType != nil {
				finalVariableType = finalInitializerType
			} else {
				// --- FIX: No annotation, no initializer -> infer Undefined ---
				finalVariableType = types.Undefined // Assign Undefined, not Any
				// Remove the error message for this valid case
				// c.addError(node.Token.Line, fmt.Sprintf("cannot infer type for variable '%s'; missing initializer or type annotation", node.Name.Value))
			}
		} else { // Have annotation, check initializer assignment
			if finalInitializerType != nil {
				if !c.isAssignable(finalInitializerType, finalVariableType) {
					c.addError(node.Token.Line, fmt.Sprintf("cannot assign type '%s' to variable '%s' of type '%s'", finalInitializerType.String(), node.Name.Value, finalVariableType.String()))
				}
				// --- Check if inferred type matches annotation if both function types ---
				// If declaredType was a func type and finalInitializerType is also a func type,
				// we might want stricter checks here later (e.g., subtype compatibility)
				// For now, isAssignable handles basic cases.
				// If prelim was used, the finalInitializerType *should* be compatible or better.
			} // else: No initializer, annotation is enough
		}

		// Check for missing initializer/annotation - This check is now redundant due to the fix above
		// if node.Value == nil && node.TypeAnnotation == nil {
		// 	 c.addError(node.Token.Line, fmt.Sprintf("cannot infer type for variable '%s'; missing initializer or type annotation", node.Name.Value))
		// }

		// 4. UPDATE variable type in the current environment with the final type
		// We use Define again which will overwrite the temporary type.
		// --- DEBUG: Check types before and after update ---
		currentType, _ := c.env.Resolve(node.Name.Value)
		fmt.Printf("// [Checker LetStmt] Updating '%s'. Current type: %s, Final type: %s\n", node.Name.Value, currentType.String(), finalVariableType.String())
		// --- END DEBUG ---
		if !c.env.Define(node.Name.Value, finalVariableType) { // Use finalVariableType
			fmt.Printf("// [Checker LetStmt] WARNING: Re-Define failed unexpectedly for '%s'\n", node.Name.Value)
		}
		// --- DEBUG: Check type after update ---
		updatedType, _ := c.env.Resolve(node.Name.Value)
		fmt.Printf("// [Checker LetStmt] Updated '%s'. Type after update: %s\n", node.Name.Value, updatedType.String())
		// --- END DEBUG ---

		// Set computed type on the Name Identifier node itself
		c.SetComputedType(node.Name, finalVariableType)

	case *parser.ConstStatement:
		// --- UPDATED: Handle ConstStatement (Very similar to LetStatement) ---
		// 1. Handle Type Annotation (if present)
		var declaredType types.Type
		if node.TypeAnnotation != nil {
			declaredType = c.resolveTypeAnnotation(node.TypeAnnotation)
		} else {
			declaredType = nil // Indicates type inference is needed
		}

		// 2. Handle Initializer (Must be present for const)
		var initializerType types.Type
		if node.Value != nil {
			c.visit(node.Value) // Compute type of the initializer
			initializerType = c.GetComputedTypeOrAny(node.Value)
		} else {
			// Constants MUST be initialized
			c.addError(node.Token.Line, fmt.Sprintf("const declaration '%s' must be initialized", node.Name.Value))
			initializerType = types.Any // Assign Any to prevent further cascading errors downstream
		}

		// 3. Determine the final type and check assignability
		finalType := declaredType
		if finalType == nil { // Infer from initializer
			finalType = initializerType // Since initializer is mandatory
		} else { // Have annotation, check initializer assignment
			if !c.isAssignable(initializerType, finalType) {
				c.addError(node.Token.Line, fmt.Sprintf("cannot assign type '%s' to constant '%s' of type '%s'", initializerType.String(), node.Name.Value, finalType.String()))
			}
		}

		// 4. Define variable in the current environment
		if !c.env.Define(node.Name.Value, finalType) {
			c.addError(node.Name.Token.Line, fmt.Sprintf("constant '%s' already declared in this scope", node.Name.Value))
		}
		// Set computed type on the Name Identifier node itself
		c.SetComputedType(node.Name, finalType)

	case *parser.ReturnStatement:
		// --- UPDATED: Handle ReturnStatement ---
		var actualReturnType types.Type = types.Undefined // Default if no return value
		if node.ReturnValue != nil {
			c.visit(node.ReturnValue)
			actualReturnType = c.GetComputedTypeOrAny(node.ReturnValue)
		}

		// Check against expected type if available
		if c.currentExpectedReturnType != nil {
			if !c.isAssignable(actualReturnType, c.currentExpectedReturnType) {
				line := 0 // TODO: Get line from return value token?
				if node.ReturnValue != nil {
					// Try to get line from expression node if possible
					// line = node.ReturnValue.TokenLiteral() ... needs Token access
				}
				c.addError(line, fmt.Sprintf("cannot return type '%s'; expected type '%s'", actualReturnType.String(), c.currentExpectedReturnType.String()))
			}
		} else {
			// No explicit annotation, collect for inference
			// Ensure the slice exists (might be visiting return outside a function theoretically?)
			if c.currentInferredReturnTypes != nil {
				c.currentInferredReturnTypes = append(c.currentInferredReturnTypes, actualReturnType)
			} else {
				// This case (return outside a function) should likely be a separate error?
				// Or maybe caught by parser? For now, just ignore.
			}
		}

	case *parser.BlockStatement:
		// --- UPDATED: Handle Block Scope ---
		// --- DEBUG ---
		fmt.Printf("// [Checker Visit Block] Entering Block. Current Env: %p\n", c.env)
		if c.env == nil {
			panic("Checker env is nil before creating block scope!")
		}
		// --- END DEBUG ---
		// 1. Create a new enclosed environment
		originalEnv := c.env // Store the current environment
		c.env = NewEnclosedEnvironment(originalEnv)

		// --- DEBUG: Check node.Statements before ranging ---
		if node.Statements == nil {
			fmt.Printf("// [Checker Visit Block] WARNING: node.Statements is nil for Block %p\n", node)
		} else {
			fmt.Printf("// [Checker Visit Block] node.Statements length: %d\n", len(node.Statements))
		}
		// --- END DEBUG ---

		// 2. Visit statements within the new scope
		for i, stmt := range node.Statements { // Add index 'i' for logging
			// --- DEBUG ---
			fmt.Printf("// [Checker Visit Block Loop] Index: %d, Stmt Type: %T, Stmt Ptr: %p\n", i, stmt, stmt)
			if stmt == nil {
				fmt.Printf("// [Checker Visit Block Loop] ERROR: Stmt at index %d is nil! Skipping.\n", i)
				continue // Skip visiting nil statement
			}
			// --- END DEBUG ---
			c.visit(stmt)
		}

		// --- DEBUG ---
		fmt.Printf("// [Checker Visit Block] Exiting Block. Restoring Env: %p (from current %p)\n", originalEnv, c.env)
		if originalEnv == nil {
			panic("Checker originalEnv is nil before restoring block scope!")
		}
		// --- END DEBUG ---
		// 3. Restore the outer environment
		c.env = originalEnv

	// --- Literal Expressions ---
	case *parser.NumberLiteral:
		// Treat number literals as literal types during checking
		c.SetComputedType(node, &types.LiteralType{Value: vm.Number(node.Value)})

	case *parser.StringLiteral:
		// Treat string literals as literal types during checking
		c.SetComputedType(node, &types.LiteralType{Value: vm.String(node.Value)})

	case *parser.BooleanLiteral:
		// Treat boolean literals as literal types during checking
		c.SetComputedType(node, &types.LiteralType{Value: vm.Bool(node.Value)})

	case *parser.NullLiteral:
		c.SetComputedType(node, types.Null)

	case *parser.UndefinedLiteral:
		c.SetComputedType(node, types.Undefined)

	// --- Other Expressions ---
	case *parser.Identifier:
		// --- Check concrete pointer AFTER type switch ---
		if node == nil {
			fmt.Printf("// [Checker Debug] visit(Identifier): node is nil!\n") // DEBUG
			return
		}
		// --- Log state BEFORE potentially problematic operations ---
		fmt.Printf("// [Checker Debug] visit(Identifier): node=%p, Value='%s', Token={%s %q %d}\n", node, node.Value, node.Token.Type, node.Token.Literal, node.Token.Line) // DEBUG
		fmt.Printf("// [Checker Debug] visit(Identifier): c.env=%p\n", c.env)                                                                                               // DEBUG

		// --- UPDATED: Handle Identifier (Value Context Only) ---
		// Assume this is visited in a value context.
		// Type context identifiers are handled by resolveTypeAnnotation.

		// Safety check for incomplete nodes from parser errors - *REMOVED* (covered by check above)
		// if node == nil {
		// 	return
		// }

		typ, found := c.env.Resolve(node.Value) // Use node.Value directly
		if !found {
			fmt.Printf("// [Checker Debug] visit(Identifier): '%s' not found in env %p\n", node.Value, c.env) // DEBUG
			// Use token from node if available, else use line 0
			line := 0
			tokenValue := "<unknown>"
			// Check if Token is not the zero value before accessing Line
			if node.Token != (lexer.Token{}) {
				line = node.Token.Line
				tokenValue = node.Value // Use node.Value directly
			}
			c.addError(line, fmt.Sprintf("undefined variable: %s", tokenValue))
			// Set computed type if node itself is not nil (already checked)
			c.SetComputedType(node, types.Any) // Set to Any on error?
		} else {
			// --- DEBUG: Log raw type value immediately after resolve ---
			// fmt.Printf("// [Checker Debug] Identifier: Resolved type for '%s': Ptr=%p, Value=%#v\n", node.Value, typ, typ) // Re-commented
			// --- END DEBUG ---

			fmt.Printf("// [Checker Debug] visit(Identifier): '%s' found in env %p, type: %s\n", node.Value, c.env, typ.String()) // DEBUG - Uncommented

			// node is guaranteed non-nil here
			c.SetComputedType(node, typ)
			// --- DEBUG: Explicit panic before return ---
			// panic(fmt.Sprintf("Intentional panic after setting type for Identifier '%s'", node.Value))
			// --- END DEBUG ---
		}

	case *parser.PrefixExpression:
		// --- UPDATED: Handle PrefixExpression ---
		c.visit(node.Right) // Visit the operand first
		rightType := node.Right.GetComputedType()
		var resultType types.Type = types.Any // Default to Any on error
		line := node.Token.Line

		if rightType != nil { // Proceed only if operand type is known
			switch node.Operator {
			case "-":
				if rightType == types.Number {
					resultType = types.Number
				} else {
					c.addError(line, fmt.Sprintf("operator '%s' cannot be applied to type '%s'", node.Operator, rightType.String()))
				}
			case "!":
				// Logical NOT can be applied to any type (implicitly converts to boolean)
				resultType = types.Boolean
			default:
				c.addError(line, fmt.Sprintf("unsupported prefix operator: %s", node.Operator))
			}
		} // else: Error already reported during operand check
		c.SetComputedType(node, resultType)

	case *parser.InfixExpression:
		// --- UPDATED: Handle InfixExpression ---
		c.visit(node.Left)
		c.visit(node.Right)
		leftType := node.Left.GetComputedType()
		rightType := node.Right.GetComputedType()
		var resultType types.Type = types.Any // Default to Any on error
		line := node.Token.Line

		if leftType != nil && rightType != nil { // Proceed only if operand types are known
			switch node.Operator {
			case "+":
				// Basic rule: number + number = number
				// TODO: Handle string concatenation later
				if leftType == types.Number && rightType == types.Number {
					resultType = types.Number
				} else {
					c.addError(line, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
				}
			case "-", "*", "/":
				if leftType == types.Number && rightType == types.Number {
					resultType = types.Number
				} else {
					c.addError(line, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
				}
			case "<", ">", "<=", ">=":
				// Expect numbers for comparison for now
				if leftType == types.Number && rightType == types.Number {
					resultType = types.Boolean
				} else {
					c.addError(line, fmt.Sprintf("operator '%s' cannot be applied to types '%s' and '%s'", node.Operator, leftType.String(), rightType.String()))
				}
			case "==", "!=", "===", "!==":
				// Basic check: allow comparing same primitive types
				// TODO: More complex equality rules (null, undefined, coercion)
				if c.isAssignable(leftType, rightType) || c.isAssignable(rightType, leftType) {
					resultType = types.Boolean
				} else {
					// Warn or error on always-false comparison?
					c.addError(line, fmt.Sprintf("comparison between incompatible types '%s' and '%s'", leftType.String(), rightType.String()))
					resultType = types.Boolean // Comparison still results in a boolean
				}
			case "&&", "||":
				// Result type is complex (union of branches)
				// TODO: Implement Union types. For now, default to Any.
				resultType = types.Any
			case "??":
				// Result type is type of left if not null/undef, else type of right.
				// TODO: Implement Union types. For now, default to Any.
				resultType = types.Any
			default:
				c.addError(line, fmt.Sprintf("unsupported infix operator: %s", node.Operator))
			}
		} // else: Error already reported during operand check
		c.SetComputedType(node, resultType)

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
		c.SetComputedType(node, types.Void) // Use checker's method

	case *parser.TernaryExpression:
		// TODO: Handle TernaryExpression
		c.visit(node.Condition)
		c.visit(node.Consequence)
		c.visit(node.Alternative)

	case *parser.FunctionLiteral:
		// --- UPDATED: Handle FunctionLiteral ---
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
				} // else: error already added by resolveTypeAnnotation
			}
			paramTypes = append(paramTypes, paramType)
			paramNames = append(paramNames, param.Name)
			// Set computed type on the parameter node itself
			param.ComputedType = paramType
		}

		// 3. Resolve Explicit Return Type Annotation & Set Context
		expectedReturnType := c.resolveTypeAnnotation(node.ReturnTypeAnnotation)
		c.currentExpectedReturnType = expectedReturnType // May be nil
		c.currentInferredReturnTypes = nil               // Reset for this function
		if expectedReturnType == nil {
			// Only allocate if we need to infer
			c.currentInferredReturnTypes = []types.Type{}
		}

		// 4. Create function scope & define parameters
		// --- DEBUG ---
		fmt.Printf("// [Checker Visit FuncLit] Creating Func Scope. Current Env: %p\n", c.env)
		if c.env == nil {
			panic("Checker env is nil before creating func scope!")
		}
		// --- END DEBUG ---
		originalEnv := c.env
		funcEnv := NewEnclosedEnvironment(originalEnv)
		c.env = funcEnv
		for i, nameNode := range paramNames {
			if !funcEnv.Define(nameNode.Value, paramTypes[i]) {
				// This shouldn't happen if parser prevents duplicate param names
				c.addError(nameNode.Token.Line, fmt.Sprintf("duplicate parameter name: %s", nameNode.Value))
			}
		}

		// --- FIX for Recursion: Define function name in its own scope BEFORE visiting body ---
		var initialFuncType *types.FunctionType
		if node.Name != nil {
			initialFuncType = &types.FunctionType{
				ParameterTypes: paramTypes,         // We know param types already
				ReturnType:     expectedReturnType, // Use explicit annotation if present, else nil
			}
			if !funcEnv.Define(node.Name.Value, initialFuncType) {
				// This might happen if a param has the same name as the function
				c.addError(node.Name.Token.Line, fmt.Sprintf("identifier '%s' already declared in this scope (parameter or function name conflict)", node.Name.Value))
			}
			fmt.Printf("// [Checker Visit FuncLit] Defined self '%s' in INNER Env: %p with initial type %s\n", node.Name.Value, funcEnv, initialFuncType.String()) // DEBUG
		}
		// --- END FIX ---

		// 5. Visit Body
		c.visit(node.Body)

		// 6. Determine Final Return Type (Inference)
		var finalReturnType types.Type = expectedReturnType // USE INTERFACE TYPE
		if finalReturnType == nil {                         // If no explicit annotation, infer
			if len(c.currentInferredReturnTypes) == 0 {
				finalReturnType = types.Undefined // Treat as void/undefined
			} else {
				// --- FIX: Improve inference for multiple returns ---
				if len(c.currentInferredReturnTypes) == 1 {
					finalReturnType = c.currentInferredReturnTypes[0]
				} else {
					firstType := c.currentInferredReturnTypes[0]
					allSame := true
					for _, typ := range c.currentInferredReturnTypes[1:] {
						// TODO: Use proper type equality check later
						if typ != firstType {
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
				// --- END FIX ---
			}
		}
		if finalReturnType == nil { // Should not happen, but safety check
			finalReturnType = types.Any
		}

		// 7. Create or Update FunctionType
		// Reuse the initial type if created, otherwise create new
		var funcType *types.FunctionType
		if initialFuncType != nil {
			funcType = initialFuncType
			funcType.ReturnType = finalReturnType // Update with the final inferred/checked type
		} else {
			funcType = &types.FunctionType{
				ParameterTypes: paramTypes,
				ReturnType:     finalReturnType,
			}
		}

		// 8. Set ComputedType on the FunctionLiteral node
		c.SetComputedType(node, funcType)

		// 9. Define named function in the *outer* scope (using final type)
		if node.Name != nil {
			// --- DEBUG ---
			fmt.Printf("// [Checker Visit FuncLit] Defining func '%s' in OUTER Env: %p with final type %s\n", node.Name.Value, originalEnv, funcType.String())
			if originalEnv == nil {
				panic("Checker originalEnv is nil before defining named func!")
			}
			// --- END DEBUG ---
			if !originalEnv.Define(node.Name.Value, funcType) { // Use the final funcType here
				c.addError(node.Name.Token.Line, fmt.Sprintf("function '%s' already declared in this scope", node.Name.Value))
			}
		}

		// 10. Restore outer environment and context
		// --- DEBUG ---
		fmt.Printf("// [Checker Visit FuncLit] Exiting Func. Restoring Env: %p (from current %p)\n", originalEnv, c.env)
		if originalEnv == nil {
			panic("Checker originalEnv is nil before restoring func scope!")
		}
		// --- END DEBUG ---
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
		fmt.Printf("// [Checker Visit ArrowFunc] Creating Func Scope. Current Env: %p\n", c.env)
		if c.env == nil {
			panic("Checker env is nil before creating arrow scope!")
		}
		// --- END DEBUG ---
		originalEnv := c.env
		funcEnv := NewEnclosedEnvironment(originalEnv)
		c.env = funcEnv
		for i, nameNode := range paramNames {
			if !funcEnv.Define(nameNode.Value, paramTypes[i]) {
				c.addError(nameNode.Token.Line, fmt.Sprintf("duplicate parameter name: %s", nameNode.Value))
			}
		}

		// 5. Visit Body
		c.visit(node.Body)
		var bodyType types.Type = types.Any
		isExprBody := false
		// Special handling for expression body
		if exprBody, ok := node.Body.(parser.Expression); ok {
			isExprBody = true
			bodyType = c.GetComputedTypeOrAny(exprBody)
			// If body is an expression, its type *is* the single inferred return type,
			// unless overridden by an annotation.
			if c.currentInferredReturnTypes != nil { // Only append if inference is active
				c.currentInferredReturnTypes = append(c.currentInferredReturnTypes, bodyType)
			}

			// --- NEW: Check expression body type against annotation ---
			if expectedReturnType != nil {
				if !c.isAssignable(bodyType, expectedReturnType) {
					// TODO: Get line number from exprBody token if possible
					c.addError(node.Token.Line, fmt.Sprintf("cannot return expression of type '%s' from arrow function with return type annotation '%s'", bodyType.String(), expectedReturnType.String()))
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
		fmt.Printf("// [Checker ArrowFunc] Computed funcType: %s\n", funcType.String())
		// --- END DEBUG ---

		// 8. Set ComputedType on the ArrowFunctionLiteral node
		c.SetComputedType(node, funcType)

		// 9. Restore outer environment and context
		// --- DEBUG ---
		fmt.Printf("// [Checker Visit ArrowFunc] Exiting Arrow Func. Restoring Env: %p (from current %p)\n", originalEnv, c.env)
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
		// --- DEBUG: Check if we return here ---
		fmt.Printf("// [Checker CallExpr] Returned from visiting node.Function: %T\n", node.Function)
		// --- END DEBUG ---
		funcNodeType, ok := c.GetComputedType(node.Function) // Retrieve type stored by the identifier visit
		if !ok {
			// Error visiting the function expression itself? Or type not found?
			funcIdent, isIdent := node.Function.(*parser.Identifier)
			errMsg := "cannot determine type of called expression"
			if isIdent {
				errMsg = fmt.Sprintf("cannot determine type of called identifier '%s'", funcIdent.Value)
			}
			c.addError(node.Token.Line, errMsg)
			c.SetComputedType(node, types.Any)
			return
		}

		// allow calling any for now
		if funcNodeType == types.Any {
			c.SetComputedType(node, types.Any)
			return
		}

		// Assert that the computed type is actually a function type
		funcType, ok := funcNodeType.(*types.FunctionType)
		if !ok {
			c.addError(node.Token.Line, fmt.Sprintf("cannot call value of type '%s'", funcNodeType.String()))
			c.SetComputedType(node, types.Any) // Result type is unknown/error
			return
		}

		// 2. Check Arity (Number of arguments)
		expectedArgCount := len(funcType.ParameterTypes)
		actualArgCount := len(node.Arguments)
		if actualArgCount != expectedArgCount {
			c.addError(node.Token.Line, fmt.Sprintf("expected %d arguments, but got %d", expectedArgCount, actualArgCount))
			// Continue checking assignable args anyway? Or stop?
			// Let's stop checking args if arity is wrong, but still set return type.
			c.SetComputedType(node, funcType.ReturnType)
			return
		}

		// 3. Check Argument Types
		for i, argNode := range node.Arguments {
			c.visit(argNode) // Visit argument to compute its type
			argType := c.GetComputedTypeOrAny(argNode)
			paramType := funcType.ParameterTypes[i]

			if argType == nil {
				// Error computing argument type, can't check assignability
				// Error already added, just skip assignability check for this arg.
				continue
			}

			if !c.isAssignable(argType, paramType) {
				// --- FIXED: Extract line number from argument node ---
				argLine := 0 // Default line number
				switch n := argNode.(type) {
				// List common node types that have a Token field
				case *parser.Identifier:
					argLine = n.Token.Line
				case *parser.NumberLiteral:
					argLine = n.Token.Line
				case *parser.StringLiteral:
					argLine = n.Token.Line
				case *parser.BooleanLiteral:
					argLine = n.Token.Line
				case *parser.NullLiteral:
					argLine = n.Token.Line
				case *parser.UndefinedLiteral:
					argLine = n.Token.Line
				case *parser.PrefixExpression:
					argLine = n.Token.Line // Token is operator
				case *parser.InfixExpression:
					argLine = n.Token.Line // Token is operator
				case *parser.CallExpression:
					argLine = n.Token.Line // Token is '('
				case *parser.FunctionLiteral:
					argLine = n.Token.Line // Token is 'function'
				case *parser.ArrowFunctionLiteral:
					argLine = n.Token.Line // Token is '=>'
				// Add other expression types holding a relevant Token here...
				default:
					// Fallback or attempt to get token via interface if Node had GetToken()
					// For now, default 0 is kept if type assertion fails
					fmt.Printf("// [Checker Warning] Could not get specific line number for arg type %T\n", argNode)
				}
				// --- End Fix ---
				c.addError(argLine, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'", i+1, argType.String(), paramType.String()))
				// Continue checking other arguments even if one fails
			}
		}

		// 4. Set Result Type
		// If function returns 'never', maybe the call expression type should also be 'never'?
		// if funcType.ReturnType == types.Never { ... }
		c.SetComputedType(node, funcType.ReturnType)

	case *parser.AssignmentExpression:
		// TODO: Handle AssignmentExpression
		c.visit(node.Left)
		c.visit(node.Value)

	case *parser.UpdateExpression:
		// TODO: Handle UpdateExpression
		c.visit(node.Argument)

	// --- Loop Statements (Control flow, check condition/body) ---
	case *parser.WhileStatement:
		c.visit(node.Condition)
		c.visit(node.Body)

	case *parser.DoWhileStatement:
		c.visit(node.Body)
		c.visit(node.Condition)

	case *parser.ForStatement:
		c.visit(node.Initializer)
		c.visit(node.Condition)
		c.visit(node.Update)
		c.visit(node.Body)

	// --- Loop Control (No specific type checking needed?) ---
	case *parser.BreakStatement:
		break // Nothing to check type-wise
	case *parser.ContinueStatement:
		break // Nothing to check type-wise

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
func (c *Checker) addError(line int, message string) {
	// TODO: Get line number from token if available
	c.errors = append(c.errors, TypeError{Line: line, Message: message})
}

// --- Type Helper Functions ---

// SetComputedType associates a computed type with an AST node.
func (c *Checker) SetComputedType(node parser.Node, typ types.Type) {
	if node == nil {
		return // Or panic? Avoid polluting map with nil nodes.
	}
	c.computedTypes[node] = typ
}

// GetComputedType retrieves the computed type associated with an AST node.
// Returns the type and true if found, otherwise nil and false.
func (c *Checker) GetComputedType(node parser.Node) (types.Type, bool) {
	typ, ok := c.computedTypes[node]
	return typ, ok
}

// GetComputedTypeOrAny retrieves the computed type or returns types.Any if none is found.
func (c *Checker) GetComputedTypeOrAny(node parser.Node) types.Type {
	if typ, ok := c.GetComputedType(node); ok {
		return typ
	}
	return types.Any // Default to Any if no type was computed/stored
}

// --- NEW: Type Alias Statement Check ---

func (c *Checker) checkTypeAliasStatement(node *parser.TypeAliasStatement) {
	// 1. Resolve the type expression on the right-hand side
	aliasedType := c.resolveTypeAnnotation(node.Type)
	if aliasedType == nil {
		// Error already added by resolveTypeAnnotation or invalid type expr
		// We could add another error here, but maybe redundant.
		// fmt.Printf("// [Checker TypeAlias] Failed to resolve type for alias '%s'\n", node.Name.Value)
		return // Cannot define alias if type resolution failed
	}

	// 2. Define the alias in the current environment
	if !c.env.DefineTypeAlias(node.Name.Value, aliasedType) {
		c.addError(node.Name.Token.Line, fmt.Sprintf("type alias name '%s' conflicts with an existing variable or type alias in this scope", node.Name.Value))
	}

	// TODO: Add cycle detection? (e.g., type A = B; type B = A;)
	// TODO: Set computed type on the node itself? Maybe not necessary for aliases.

	fmt.Printf("// [Checker TypeAlias] Defined alias '%s' as type '%s' in env %p\n", node.Name.Value, aliasedType.String(), c.env)

}

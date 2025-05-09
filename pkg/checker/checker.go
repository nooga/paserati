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

// --- NEW: Symbol Information ---
type SymbolInfo struct {
	Type    types.Type
	IsConst bool
}

// Environment manages type information within scopes.
type Environment struct {
	symbols     map[string]SymbolInfo // UPDATED: Stores SymbolInfo (type + const status)
	typeAliases map[string]types.Type // Stores resolved types for type aliases
	outer       *Environment          // Pointer to the enclosing environment
}

// NewEnvironment creates a new top-level type environment.
func NewEnvironment() *Environment {
	return &Environment{
		symbols:     make(map[string]SymbolInfo), // Initialize with SymbolInfo
		typeAliases: make(map[string]types.Type), // Initialize
		outer:       nil,
	}
}

// NewEnclosedEnvironment creates a new environment nested within an outer one.
func NewEnclosedEnvironment(outer *Environment) *Environment {
	return &Environment{
		symbols:     make(map[string]SymbolInfo), // Initialize with SymbolInfo
		typeAliases: make(map[string]types.Type), // Initialize
		outer:       outer,
	}
}

// Define adds a new *variable* type binding and its const status to the current environment scope.
// Returns false if the name conflicts with an existing variable/const in this scope.
func (e *Environment) Define(name string, typ types.Type, isConst bool) bool {
	// Check for conflict with existing variable/constant in this scope
	if _, exists := e.symbols[name]; exists {
		return false // Name already taken by a variable/const
	}
	// Check for conflict with existing type alias in this scope
	if _, exists := e.typeAliases[name]; exists {
		return false // Name already taken by a type alias
	}
	e.symbols[name] = SymbolInfo{Type: typ, IsConst: isConst}
	return true
}

// --- NEW: Update method ---

// Update modifies the type of an *existing* variable symbol in the current environment scope.
// It does NOT change the IsConst status.
// Returns true if the symbol was found and updated, false otherwise.
func (e *Environment) Update(name string, typ types.Type) bool {
	info, exists := e.symbols[name]
	if !exists {
		return false // Symbol not found in this scope
	}
	// Update the type, keep the original IsConst status
	e.symbols[name] = SymbolInfo{Type: typ, IsConst: info.IsConst}
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
// Returns the type, whether it's constant, and true if found. Otherwise returns nil, false, false.
func (e *Environment) Resolve(name string) (typ types.Type, isConst bool, found bool) {
	// --- DEBUG ---
	if checkerDebug {
		debugPrintf("// [Env Resolve] env=%p, name='%s', outer=%p\n", e, name, e.outer) // Log entry
	}
	if e == nil {
		debugPrintf("// [Env Resolve] ERROR: Attempted to resolve '%s' on nil environment!\n", name)
		// Prevent panic, but this indicates a bug elsewhere.
		return nil, false, false
	}
	if e.symbols == nil {
		debugPrintf("// [Env Resolve] ERROR: env %p has nil symbols map!\n", e)
		// Prevent panic, indicate bug.
		return nil, false, false
	}
	// --- END DEBUG ---

	// Check current scope first
	info, ok := e.symbols[name]
	if ok {
		debugPrintf("// [Env Resolve] Found '%s' in env %p\n", name, e) // DEBUG
		return info.Type, info.IsConst, true                            // Return type, const status, and found=true
	}

	// If not found and there's an outer scope, check there recursively
	if e.outer != nil {
		debugPrintf("// [Env Resolve] '%s' not in env %p, checking outer %p...\n", name, e, e.outer) // DEBUG
		return e.outer.Resolve(name)                                                                 // Propagate results from outer scope
	}

	// Not found in any scope
	debugPrintf("// [Env Resolve] '%s' not found in env %p (no outer)\n", name, e) // DEBUG
	return nil, false, false                                                       // Return nil type, isConst=false, found=false
}

// ResolveType looks up a *type name* (could be alias or primitive) in the current environment and its outer scopes.
// Returns the resolved type and true if found, otherwise nil and false.
func (e *Environment) ResolveType(name string) (types.Type, bool) {
	// --- DEBUG ---
	debugPrintf("// [Env ResolveType] env=%p, name='%s', outer=%p\n", e, name, e.outer)
	if e == nil {
		return nil, false
	} // Safety
	if e.typeAliases == nil {
		debugPrintf("// [Env ResolveType] ERROR: env %p has nil typeAliases map!\n", e)
		return nil, false
	}
	// --- END DEBUG ---

	// 1. Check type aliases in current scope
	typ, ok := e.typeAliases[name]
	if ok {
		debugPrintf("// [Env ResolveType] Found alias '%s' in env %p\n", name, e)
		return typ, true
	}

	// 2. If not found in current aliases, check outer scopes recursively
	if e.outer != nil {
		debugPrintf("// [Env ResolveType] Alias '%s' not in env %p, checking outer %p...\n", name, e, e.outer)
		return e.outer.ResolveType(name)
	}

	// 3. If not found in any alias scope, check built-in primitives (only at global level?)
	//    (This check is actually done in the Checker's resolveTypeAnnotation after trying env.ResolveType)

	debugPrintf("// [Env ResolveType] Alias '%s' not found in env %p (no outer)\n", name, e)
	return nil, false
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
func (c *Checker) Check(program *parser.Program) []errors.PaseratiError { // Changed return type
	c.program = program
	c.errors = []errors.PaseratiError{} // Reset errors

	// Start traversal from the program root
	c.visit(program)

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
		var computedInitializerType types.Type // Type computed directly from initializer visit
		if node.Value != nil {
			c.visit(node.Value)                                    // Compute type of the initializer
			computedInitializerType = node.Value.GetComputedType() // <<< USE NODE METHOD
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
				finalVariableType = types.GetWidenedType(computedInitializerType)
			} else {
				finalVariableType = types.Undefined // No annotation, no initializer
			}
		}

		// Safety fallback if type determination failed somehow
		if finalVariableType == nil {
			debugPrintf("// [Checker LetStmt] WARNING: finalVariableType is nil for '%s', falling back to Any\n", node.Name.Value)
			finalVariableType = types.Any
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
		node.Name.SetComputedType(finalVariableType) // <<< USE NODE METHOD

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
		var finalType types.Type

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
			finalType = types.GetWidenedType(computedInitializerType)
		}

		// Safety fallback if type determination failed somehow
		if finalType == nil {
			debugPrintf("// [Checker ConstStmt] WARNING: finalType is nil for '%s', falling back to Any\n", node.Name.Value)
			finalType = types.Any
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
		// --- UPDATED: Handle Block Scope ---
		// --- DEBUG ---
		debugPrintf("// [Checker Visit Block] Entering Block. Current Env: %p\n", c.env)
		if c.env == nil {
			panic("Checker env is nil before creating block scope!")
		}
		// --- END DEBUG ---
		// 1. Create a new enclosed environment
		originalEnv := c.env // Store the current environment
		c.env = NewEnclosedEnvironment(originalEnv)

		// --- DEBUG: Check node.Statements before ranging ---
		if node.Statements == nil {
			debugPrintf("// [Checker Visit Block] WARNING: node.Statements is nil for Block %p\n", node)
		} else {
			debugPrintf("// [Checker Visit Block] node.Statements length: %d\n", len(node.Statements))
		}
		// --- END DEBUG ---

		// 2. Visit statements within the new scope
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
		// 3. Restore the outer environment
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
		debugPrintf("// [Checker Visit FuncLit] Creating Func Scope. Current Env: %p\n", c.env)
		if c.env == nil {
			panic("Checker env is nil before creating func scope!")
		}
		// --- END DEBUG ---
		originalEnv := c.env
		funcEnv := NewEnclosedEnvironment(originalEnv)
		c.env = funcEnv
		for i, nameNode := range paramNames {
			if !funcEnv.Define(nameNode.Value, paramTypes[i], false) {
				// This shouldn't happen if parser prevents duplicate param names
				c.addError(nameNode, fmt.Sprintf("duplicate parameter name: %s", nameNode.Value))
			}
		}

		// --- FIX for Recursion: Define function name in its own scope BEFORE visiting body ---
		var initialFuncType *types.FunctionType
		if node.Name != nil {
			initialFuncType = &types.FunctionType{
				ParameterTypes: paramTypes,         // We know param types already
				ReturnType:     expectedReturnType, // Use explicit annotation if present, else nil
			}
			if !funcEnv.Define(node.Name.Value, initialFuncType, false) {
				// This might happen if a param has the same name as the function
				c.addError(node.Name, fmt.Sprintf("identifier '%s' already declared in this scope (parameter or function name conflict)", node.Name.Value))
			}
			debugPrintf("// [Checker Visit FuncLit] Defined self '%s' in INNER Env: %p with initial type %s\n", node.Name.Value, funcEnv, initialFuncType.String()) // DEBUG
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
		node.SetComputedType(funcType)

		// 9. Define named function in the *outer* scope (using final type)
		if node.Name != nil {
			// --- DEBUG ---
			debugPrintf("// [Checker Visit FuncLit] Defining func '%s' in OUTER Env: %p with final type %s\n", node.Name.Value, originalEnv, funcType.String())
			if originalEnv == nil {
				panic("Checker originalEnv is nil before defining named func!")
			}
			// --- END DEBUG ---
			if !originalEnv.Define(node.Name.Value, funcType, false) { // Use the final funcType here
				c.addError(node.Name, fmt.Sprintf("function '%s' already declared in this scope", node.Name.Value))
			}
		}

		// 10. Restore outer environment and context
		// --- DEBUG ---
		debugPrintf("// [Checker Visit FuncLit] Exiting Func. Restoring Env: %p (from current %p)\n", originalEnv, c.env)
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
		// --- DEBUG: Check if we return here ---
		debugPrintf("// [Checker CallExpr] Returned from visiting node.Function: %T\n", node.Function)
		// --- END DEBUG ---
		funcNodeType := node.Function.GetComputedType() // Retrieve type stored by the identifier visit
		if funcNodeType == nil {
			// Error visiting the function expression itself? Or type not found?
			funcIdent, isIdent := node.Function.(*parser.Identifier)
			errMsg := "cannot determine type of called expression"
			if isIdent {
				errMsg = fmt.Sprintf("cannot determine type of called identifier '%s'", funcIdent.Value)
			}
			c.addError(node, errMsg)
			node.SetComputedType(types.Any)
			return
		}

		// allow calling any for now
		if funcNodeType == types.Any {
			node.SetComputedType(types.Any)
			return
		}

		// Assert that the computed type is actually a function type
		funcType, ok := funcNodeType.(*types.FunctionType)
		if !ok {
			c.addError(node, fmt.Sprintf("cannot call value of type '%s'", funcNodeType.String()))
			node.SetComputedType(types.Any) // Result type is unknown/error
			return
		}

		// 2. Check Arity (Number of arguments)
		expectedArgCount := len(funcType.ParameterTypes)
		actualArgCount := len(node.Arguments)
		if actualArgCount != expectedArgCount {
			c.addError(node, fmt.Sprintf("expected %d arguments, but got %d", expectedArgCount, actualArgCount))
			// Continue checking assignable args anyway? Or stop?
			// Let's stop checking args if arity is wrong, but still set return type.
			node.SetComputedType(funcType.ReturnType)
			return
		}

		// 3. Check Argument Types
		for i, argNode := range node.Arguments {
			c.visit(argNode) // Visit argument to compute its type
			argType := argNode.GetComputedType()
			paramType := funcType.ParameterTypes[i]

			if argType == nil {
				// Error computing argument type, can't check assignability
				// Error already added, just skip assignability check for this arg.
				continue
			}

			if !c.isAssignable(argType, paramType) {
				c.addError(argNode, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'", i+1, argType.String(), paramType.String()))
				// Continue checking other arguments even if one fails
			}
		}

		// 4. Set Result Type
		// If function returns 'never', maybe the call expression type should also be 'never'?
		// if funcType.ReturnType == types.Never { ... }
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
	// 1. Resolve the type expression on the right-hand side
	aliasedType := c.resolveTypeAnnotation(node.Type)
	if aliasedType == nil {
		// Error already added by resolveTypeAnnotation or invalid type expr
		// We could add another error here, but maybe redundant.
		// debugPrintf("// [Checker TypeAlias] Failed to resolve type for alias '%s'\n", node.Name.Value)
		return // Cannot define alias if type resolution failed
	}

	// 2. Define the alias in the current environment
	if !c.env.DefineTypeAlias(node.Name.Value, aliasedType) {
		c.addError(node.Name, fmt.Sprintf("type alias name '%s' conflicts with an existing variable or type alias in this scope", node.Name.Value))
	}

	// TODO: Add cycle detection? (e.g., type A = B; type B = A;)
	// TODO: Set computed type on the node itself? Maybe not necessary for aliases.

	debugPrintf("// [Checker TypeAlias] Defined alias '%s' as type '%s' in env %p\n", node.Name.Value, aliasedType.String(), c.env)

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

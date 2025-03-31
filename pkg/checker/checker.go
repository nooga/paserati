package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
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
	symbols map[string]types.Type // Stores type bindings for the current scope
	outer   *Environment          // Pointer to the enclosing environment
}

// NewEnvironment creates a new top-level type environment.
func NewEnvironment() *Environment {
	return &Environment{
		symbols: make(map[string]types.Type),
		outer:   nil, // No outer scope for the global environment
	}
}

// NewEnclosedEnvironment creates a new environment nested within an outer one.
func NewEnclosedEnvironment(outer *Environment) *Environment {
	return &Environment{
		symbols: make(map[string]types.Type),
		outer:   outer,
	}
}

// Define adds a new type binding to the current environment scope.
// It returns false if the name is already defined *in this specific scope*.
func (e *Environment) Define(name string, typ types.Type) bool {
	if _, exists := e.symbols[name]; exists {
		return false // Already defined in this scope
	}
	e.symbols[name] = typ
	return true
}

// Resolve looks up a name in the current environment and its outer scopes.
// Returns the type and true if found, otherwise nil and false.
func (e *Environment) Resolve(name string) (types.Type, bool) {
	// Check current scope first
	typ, ok := e.symbols[name]
	if ok {
		return typ, true
	}

	// If not found and there's an outer scope, check there recursively
	if e.outer != nil {
		return e.outer.Resolve(name)
	}

	// Not found in any scope
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
}

// NewChecker creates a new type checker.
func NewChecker() *Checker {
	return &Checker{
		env:    NewEnvironment(), // Start with a global environment
		errors: []TypeError{},
		// Initialize function context fields to nil/empty
		currentExpectedReturnType:  nil,
		currentInferredReturnTypes: nil,
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

	ident, ok := node.(*parser.Identifier)
	if !ok {
		// We only handle simple identifier types for now.
		line := 0 // TODO: Get line number from node token
		c.addError(line, fmt.Sprintf("unsupported type annotation syntax: expected identifier, got %T", node))
		return nil // Indicate error
	}

	// Check against known primitive type names
	switch ident.Value {
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
	default:
		// TODO: Look up in custom type registry/environment later
		line := ident.Token.Line
		c.addError(line, fmt.Sprintf("unknown type name: %s", ident.Value))
		return nil // Indicate error
	}
}

// isAssignable checks if a value of type `source` can be assigned to a variable
// of type `target`.
// TODO: Expand significantly for structural typing, unions, intersections etc.
func isAssignable(source, target types.Type) bool {
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

	// TODO: Handle null/undefined assignability based on strict flags later.
	// For now, let's be strict unless target is Any/Unknown.
	if source == types.Null && target != types.Null {
		return false
	}
	if source == types.Undefined && target != types.Undefined {
		return false
	}

	// Check for identical types (using pointer equality for primitives)
	if source == target {
		return true
	}

	// TODO: Add structural checks for objects/arrays
	// TODO: Add checks for function type compatibility
	// TODO: Add checks for unions/intersections

	// Default: not assignable
	return false
}

// visit is the main dispatch method for AST traversal (Visitor pattern lite).
func (c *Checker) visit(node parser.Node) {
	// Handle nil nodes gracefully (e.g., from parsing errors)
	if node == nil {
		return
	}

	// Dispatch based on node type
	switch node := node.(type) {
	// --- Program ---
	case *parser.Program:
		for _, stmt := range node.Statements {
			c.visit(stmt)
		}

	// --- Statements ---
	case *parser.ExpressionStatement:
		c.visit(node.Expression)
		// TODO: Check if expression result is used? (e.g., void context)

	case *parser.LetStatement:
		// --- UPDATED: Handle LetStatement ---
		var declaredType types.Type = types.Any // Default to Any, use interface type

		// 1. Resolve TypeAnnotation to types.Type
		if node.TypeAnnotation != nil {
			resolvedAnnotationType := c.resolveTypeAnnotation(node.TypeAnnotation)
			if resolvedAnnotationType == nil {
				// Error resolving annotation, stop processing this statement
				return
			}
			declaredType = resolvedAnnotationType // Assign interface to interface
		}

		// 2. Visit Value expression to infer its type
		var initializerType types.Type
		if node.Value != nil {
			c.visit(node.Value)
			initializerType = node.Value.GetComputedType()
			if initializerType == nil {
				initializerType = types.Any
			}
		}

		// 3. Determine final type and check assignability
		if node.TypeAnnotation != nil {
			// Annotation present
			if node.Value != nil {
				// Both annotation and initializer: Check assignability
				if !isAssignable(initializerType, declaredType) {
					line := 0 // TODO: Get line from node.Value token
					c.addError(line, fmt.Sprintf("cannot assign type '%s' to variable with type '%s'", initializerType.String(), declaredType.String()))
				}
			} // else: Annotation only, declaredType already set
		} else {
			// No annotation
			if node.Value != nil {
				// Initializer only: Infer type
				declaredType = initializerType // Assign interface to interface
			} // else: Neither annotation nor initializer: declaredType defaults to Any
		}

		// Handle `let x;` case specifically -> should be undefined? Or Any? TS uses Any.
		if node.TypeAnnotation == nil && node.Value == nil {
			declaredType = types.Any // Explicitly set to Any for `let x;`
		}

		// 4. Define Name in environment with the final determined type
		if !c.env.Define(node.Name.Value, declaredType) {
			c.addError(node.Name.Token.Line, fmt.Sprintf("variable '%s' already declared in this scope", node.Name.Value))
		}

		// 5. Set the statement's computed type (useful for the declaration itself? maybe not)
		node.ComputedType = declaredType

	case *parser.ConstStatement:
		// --- UPDATED: Handle ConstStatement (Similar to Let, but must have initializer) ---
		var declaredType types.Type = types.Any // Default, use interface type

		// 1. Resolve TypeAnnotation
		if node.TypeAnnotation != nil {
			resolvedAnnotationType := c.resolveTypeAnnotation(node.TypeAnnotation)
			if resolvedAnnotationType == nil {
				return
			}
			declaredType = resolvedAnnotationType // Assign interface to interface
		}

		// 2. Visit Value expression (Const MUST have a value)
		if node.Value == nil {
			// This should be caught by the parser, but double-check.
			c.addError(node.Token.Line, fmt.Sprintf("const declaration '%s' must be initialized", node.Name.Value))
			return
		}
		c.visit(node.Value)
		initializerType := node.Value.GetComputedType()
		if initializerType == nil {
			initializerType = types.Any // Default to Any if inference failed
		}

		// 3. Determine final type and check assignability
		if node.TypeAnnotation != nil {
			// Annotation present: Check assignability
			if !isAssignable(initializerType, declaredType) {
				line := 0 // TODO: Get line from node.Value token
				c.addError(line, fmt.Sprintf("cannot assign type '%s' to const variable with type '%s'", initializerType.String(), declaredType.String()))
			}
		} else {
			// No annotation: Infer type from initializer
			declaredType = initializerType // Assign interface to interface
		}

		// 4. Define Name in environment
		if !c.env.Define(node.Name.Value, declaredType) {
			c.addError(node.Name.Token.Line, fmt.Sprintf("const variable '%s' already declared in this scope", node.Name.Value))
		}

		// 5. Set statement's computed type
		node.ComputedType = declaredType

	case *parser.ReturnStatement:
		// --- UPDATED: Handle ReturnStatement ---
		var actualReturnType types.Type = types.Undefined // Default if no return value
		if node.ReturnValue != nil {
			c.visit(node.ReturnValue)
			actualReturnType = node.ReturnValue.GetComputedType()
			if actualReturnType == nil {
				// Error during value inference, treat as Any to avoid cascading errors?
				actualReturnType = types.Any
			}
		}

		// Check against expected type if available
		if c.currentExpectedReturnType != nil {
			if !isAssignable(actualReturnType, c.currentExpectedReturnType) {
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
		// 1. Create a new enclosed environment
		originalEnv := c.env // Store the current environment
		c.env = NewEnclosedEnvironment(originalEnv)

		// 2. Visit statements within the new scope
		for _, stmt := range node.Statements {
			c.visit(stmt)
		}

		// 3. Restore the outer environment
		c.env = originalEnv

	// --- Literal Expressions ---
	case *parser.NumberLiteral:
		node.SetComputedType(types.Number)

	case *parser.StringLiteral:
		node.SetComputedType(types.String)

	case *parser.BooleanLiteral:
		node.SetComputedType(types.Boolean)

	case *parser.NullLiteral:
		node.SetComputedType(types.Null)

	// --- Other Expressions ---
	case *parser.Identifier:
		// --- UPDATED: Handle Identifier (Value Context Only) ---
		// Assume this is visited in a value context.
		// Type context identifiers are handled by resolveTypeAnnotation.
		typ, found := c.env.Resolve(node.Value)
		if !found {
			c.addError(node.Token.Line, fmt.Sprintf("undefined variable: %s", node.Value))
			node.SetComputedType(types.Any) // Set to Any on error?
		} else {
			node.SetComputedType(typ)
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
		node.SetComputedType(resultType)

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
				if isAssignable(leftType, rightType) || isAssignable(rightType, leftType) {
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
		node.SetComputedType(resultType)

	case *parser.IfExpression:
		// TODO: Handle IfExpression
		c.visit(node.Condition)
		c.visit(node.Consequence)
		c.visit(node.Alternative)

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
		originalEnv := c.env
		funcEnv := NewEnclosedEnvironment(originalEnv)
		c.env = funcEnv
		for i, nameNode := range paramNames {
			if !funcEnv.Define(nameNode.Value, paramTypes[i]) {
				// This shouldn't happen if parser prevents duplicate param names
				c.addError(nameNode.Token.Line, fmt.Sprintf("duplicate parameter name: %s", nameNode.Value))
			}
		}

		// 5. Visit Body
		c.visit(node.Body)

		// 6. Determine Final Return Type (Inference)
		var finalReturnType types.Type = expectedReturnType // USE INTERFACE TYPE
		if finalReturnType == nil {                         // If no explicit annotation, infer
			if len(c.currentInferredReturnTypes) == 0 {
				finalReturnType = types.Undefined // Treat as void/undefined
			} else {
				// TODO: Find best common type. For now, use first or Any if multiple distinct.
				finalReturnType = c.currentInferredReturnTypes[0] // Assign interface{} to interface{}
				for _, typ := range c.currentInferredReturnTypes[1:] {
					if typ != finalReturnType {
						finalReturnType = types.Any // Fallback to Any if types differ
						break
					}
				}
			}
		}
		if finalReturnType == nil { // Should not happen, but safety check
			finalReturnType = types.Any
		}

		// 7. Create FunctionType
		funcType := &types.FunctionType{
			ParameterTypes: paramTypes,
			ReturnType:     finalReturnType,
		}

		// 8. Set ComputedType on the FunctionLiteral node
		node.SetComputedType(funcType)

		// 9. Define named function in the *outer* scope
		if node.Name != nil {
			if !originalEnv.Define(node.Name.Value, funcType) {
				c.addError(node.Name.Token.Line, fmt.Sprintf("function '%s' already declared in this scope", node.Name.Value))
			}
		}

		// 10. Restore outer environment and context
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

		// 3. Set Context (Arrow functions *cannot* have return type annotations)
		c.currentExpectedReturnType = nil // Always infer for arrow
		c.currentInferredReturnTypes = []types.Type{}

		// 4. Create function scope & define parameters
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
		// Special handling for expression body
		if exprBody, ok := node.Body.(parser.Expression); ok {
			bodyType = exprBody.GetComputedType()
			if bodyType == nil {
				bodyType = types.Any
			}
			// If body is an expression, its type *is* the single inferred return type
			// unless the body itself contains return statements (complex case).
			// For now, assume expression body implies direct return of its value.
			c.currentInferredReturnTypes = []types.Type{bodyType}
		} // Else: Body is BlockStatement, returns handled by ReturnStatement visitor

		// 6. Determine Final Return Type (Inference)
		var finalReturnType types.Type = types.Undefined // Default for void, USE INTERFACE TYPE
		if len(c.currentInferredReturnTypes) > 0 {
			// TODO: Find best common type. For now, use first or Any if multiple distinct.
			finalReturnType = c.currentInferredReturnTypes[0] // Assign interface{} to interface{}
			for _, typ := range c.currentInferredReturnTypes[1:] {
				if typ != finalReturnType {
					finalReturnType = types.Any // Fallback to Any if types differ
					break
				}
			}
		} // else: No return statements found, stays Undefined
		if finalReturnType == nil { // Safety check
			finalReturnType = types.Any
		}

		// 7. Create FunctionType
		funcType := &types.FunctionType{
			ParameterTypes: paramTypes,
			ReturnType:     finalReturnType,
		}

		// 8. Set ComputedType on the ArrowFunctionLiteral node
		node.SetComputedType(funcType)

		// 9. Restore outer environment and context
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
			// Set result to Any and return, error was already added
			node.SetComputedType(types.Any)
			return
		}

		// Assert that the computed type is actually a function type
		funcType, ok := funcNodeType.(*types.FunctionType)
		if !ok {
			c.addError(node.Token.Line, fmt.Sprintf("cannot call value of type '%s'", funcNodeType.String()))
			node.SetComputedType(types.Any) // Result type is unknown/error
			return
		}

		// 2. Check Arity (Number of arguments)
		expectedArgCount := len(funcType.ParameterTypes)
		actualArgCount := len(node.Arguments)
		if actualArgCount != expectedArgCount {
			c.addError(node.Token.Line, fmt.Sprintf("expected %d arguments, but got %d", expectedArgCount, actualArgCount))
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

			if !isAssignable(argType, paramType) {
				argLine := 0 // TODO: Get line from argNode token
				c.addError(argLine, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'", i+1, argType.String(), paramType.String()))
				// Continue checking other arguments even if one fails
			}
		}

		// 4. Set Result Type
		// If function returns 'never', maybe the call expression type should also be 'never'?
		// if funcType.ReturnType == types.Never { ... }
		node.SetComputedType(funcType.ReturnType)

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

package compiler

import (
	"fmt"
	"math"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

func (c *Compiler) compileLetStatement(node *parser.LetStatement, hint Register) (Register, errors.PaseratiError) {
	// Process all variable declarations in the statement
	for _, declarator := range node.Declarations {
		// Set current declarator in legacy fields for backward compatibility
		node.Name = declarator.Name
		node.Value = declarator.Value
		node.ComputedType = declarator.ComputedType

		// Strict mode validation: FutureReservedWords cannot be used as variable names
		if c.chunk.IsStrict && isFutureReservedWord(declarator.Name.Value) {
			c.addError(declarator.Name, fmt.Sprintf("SyntaxError: Unexpected strict mode reserved word '%s'", declarator.Name.Value))
		}

		// debug disabled
		var valueReg Register = nilRegister
		var err errors.PaseratiError
		isValueFunc := false // Flag to track if value is a function literal

		if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
			isValueFunc = true
			// --- Handle let f = function g() {} or let f = function() {} ---
			// 1. Define the *variable name (f)* temporarily for potential recursion
			//    within the function body (e.g., recursive anonymous function).
			// debug disabled
			c.currentSymbolTable.Define(node.Name.Value, nilRegister)

			// 2. Compile the function literal body.
			//    Pass the variable name (f) as the hint for the function object's name
			//    if the function literal itself is anonymous.
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, node.Name.Value)
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return BadRegister, nil // Return nil error here, main error is tracked
			}
			// 3. Create the closure object
			closureReg := c.regAlloc.Alloc()
			// debug disabled
			c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols)

			// 4. Update the symbol table entry for the *variable name (f)* with the closure register.
			// debug disabled
			c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)

			// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure

			// The variable's value (the closure) is now set.
			// We don't need to assign to valueReg anymore for this path.

		} else if arrowFunc, ok := node.Value.(*parser.ArrowFunctionLiteral); ok {
			isValueFunc = true
			// --- Handle let f = () => {} ---
			// 1. Define the *variable name (f)* temporarily for potential recursion
			c.currentSymbolTable.Define(node.Name.Value, nilRegister)

			// 2. Compile the arrow function with variable name as the name hint
			//    Per ECMAScript spec, anonymous arrow functions infer name from variable
			funcConstIndex, freeSymbols, err := c.compileArrowFunctionWithName(arrowFunc, node.Name.Value)
			if err != nil {
				return BadRegister, nil
			}
			// 3. Create the closure object
			closureReg := c.regAlloc.Alloc()
			// Create a minimal FunctionLiteral for emitClosure
			var body *parser.BlockStatement
			if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
				body = blockBody
			} else {
				body = &parser.BlockStatement{}
			}
			minimalFuncLit := &parser.FunctionLiteral{Body: body}
			c.emitClosure(closureReg, funcConstIndex, minimalFuncLit, freeSymbols)

			// 4. Update the symbol table entry for the variable with the closure register
			c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)

		} else if classExpr, ok := node.Value.(*parser.ClassExpression); ok {
			// --- Handle let C = class {} or let C = class D {} ---
			// For anonymous class expressions, infer name from variable binding
			// For named class expressions, keep the class's own name
			if classExpr.Name == nil {
				// Anonymous class - set name from variable binding
				classExpr.Name = &parser.Identifier{
					Token: classExpr.Token,
					Value: node.Name.Value,
				}
			}
			// Now compile normally - the class will use its name (either own name or inferred)
			valueReg = c.regAlloc.Alloc()
			_, err = c.compileNode(classExpr, valueReg)
			if err != nil {
				return BadRegister, err
			}

		} else if node.Value != nil {
			// Compile other value types normally
			// Use existing predefined register if present
			targetReg := valueReg
			useSpilling := false
			var spillIdx uint16
			if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
				targetReg = sym.Register
			} else {
				// Try to allocate for the new variable (using lower threshold)
				var ok bool
				targetReg, ok = c.regAlloc.TryAllocForVariable()
				if !ok {
					// Variable register threshold reached, use spilling
					useSpilling = true
					spillIdx = c.AllocSpillSlot()
					// Allocate temp register for compilation (will be freed after)
					targetReg = c.regAlloc.Alloc()
				}
			}
			_, err = c.compileNode(node.Value, targetReg)
			if err != nil {
				return BadRegister, err
			}
			if useSpilling {
				// Store value to spill slot and free temp register
				c.emitStoreSpill(spillIdx, targetReg, node.Name.Token.Line)
				c.regAlloc.Free(targetReg)
				// Define the variable as spilled
				c.currentSymbolTable.DefineSpilled(node.Name.Value, spillIdx)
				// Skip normal valueReg assignment since we've handled it
				continue
			}
			valueReg = targetReg
		} // else: node.Value is nil (implicit undefined handled below)

		// Handle implicit undefined (`let x;`)
		if valueReg == nilRegister && !isValueFunc {
			// Check if we're in global scope first
			isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil && !c.isIndirectEval
			if isGlobalScope {
				// Global scope: allocate temp, load undefined, set global
				undefReg := c.regAlloc.Alloc()
				c.emitLoadUndefined(undefReg, node.Name.Token.Line)
				globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
				c.emitSetGlobal(globalIdx, undefReg, node.Name.Token.Line)
				c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
				c.regAlloc.Free(undefReg)
			} else {
				// Local scope: try to allocate register, fall back to spilling
				if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
					c.emitLoadUndefined(sym.Register, node.Name.Token.Line)
				} else {
					undefReg, ok := c.regAlloc.TryAllocForVariable()
					if ok {
						c.emitLoadUndefined(undefReg, node.Name.Token.Line)
						c.currentSymbolTable.Define(node.Name.Value, undefReg)
					} else {
						// Variable register threshold reached, use spilling
						spillIdx := c.AllocSpillSlot()
						tempReg := c.regAlloc.Alloc()
						c.emitLoadUndefined(tempReg, node.Name.Token.Line)
						c.emitStoreSpill(spillIdx, tempReg, node.Name.Token.Line)
						c.regAlloc.Free(tempReg)
						c.currentSymbolTable.DefineSpilled(node.Name.Value, spillIdx)
					}
				}
			}
		} else if !isValueFunc {
			// Define symbol ONLY for non-function values.
			// Function assignments were handled above by UpdateRegister.
			// debug disabled
			// Check if we're in global scope: no enclosing compiler AND no outer symbol table
			// For indirect eval, let/const should be local even at top level
			isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil && !c.isIndirectEval
			if isGlobalScope {
				// True global scope: use global variable
				globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
				c.emitSetGlobal(globalIdx, valueReg, node.Name.Token.Line)
				c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
			} else {
				// Local scope (function or enclosed block): use local symbol table
				if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
					debugPrintf("// [LetStatement] '%s' already predefined in R%d, moving from valueReg=R%d (symbolTable=%p)\n", node.Name.Value, sym.Register, valueReg, c.currentSymbolTable)
					c.emitMove(sym.Register, valueReg, node.Name.Token.Line)
				} else {
					debugPrintf("// [LetStatement] '%s' not predefined, defining in R%d (symbolTable=%p)\n", node.Name.Value, valueReg, c.currentSymbolTable)
					c.currentSymbolTable.Define(node.Name.Value, valueReg)
					// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
				}
			}
		} else {
			// Function value - check if it should be global
			// For indirect eval, let/const should be local even at top level
			isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil && !c.isIndirectEval
			if isGlobalScope {
				// Top-level function: also set as global
				globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
				// Get the closure register from the symbol table
				symbolRef, _, found := c.currentSymbolTable.Resolve(node.Name.Value)
				if found && symbolRef.Register != nilRegister {
					c.emitSetGlobal(globalIdx, symbolRef.Register, node.Name.Token.Line)
					// Update the symbol to be global
					c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
					// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
				}
			}
		}
		// Mark TDZ as initialized now that the let declaration has been processed
		c.currentSymbolTable.InitializeTDZ(node.Name.Value)
	} // end for declarator

	return BadRegister, nil
}

func (c *Compiler) compileVarStatement(node *parser.VarStatement, hint Register) (Register, errors.PaseratiError) {
	// Process all variable declarations in the statement
	for _, declarator := range node.Declarations {
		// Strict mode check: 'var arguments' and 'var eval' are forbidden in strict mode
		// This applies to all code, not just direct eval
		if c.chunk.IsStrict && declarator.Name != nil {
			if declarator.Name.Value == "arguments" || declarator.Name.Value == "eval" {
				c.addError(declarator.Name, fmt.Sprintf("SyntaxError: Unexpected eval or arguments in strict mode"))
				return BadRegister, nil
			}
		}

		// EvalDeclarationInstantiation check: In direct eval context, declaring 'var arguments'
		// may conflict with the caller's arguments binding (in non-strict mode).
		// This implements ECMAScript 19.2.1.3 step 5.d.ii.2.a - checking for binding conflicts.
		if c.callerScopeDesc != nil && declarator.Name != nil && declarator.Name.Value == "arguments" {
			// Case 1: eval in default parameter expression - conflicts with implicit 'arguments' binding
			// In default parameter scope, the function's implicit 'arguments' object is being initialized,
			// so 'var arguments' from eval would conflict with it.
			if c.callerScopeDesc.InDefaultParameterScope && c.callerScopeDesc.HasArgumentsBinding {
				c.addError(declarator.Name, "SyntaxError: 'var arguments' not allowed in direct eval in default parameter expression")
				return BadRegister, nil
			}
			// Case 2: eval in function body - only conflicts with EXPLICIT 'arguments' parameter/local
			// The implicit arguments object allows 'var arguments' to shadow it in the function body.
			for _, localName := range c.callerScopeDesc.LocalNames {
				if localName == "arguments" {
					c.addError(declarator.Name, "SyntaxError: 'var arguments' not allowed in direct eval when caller has explicit arguments binding")
					return BadRegister, nil
				}
			}
		}

		// Hoist function declarations in var statements similar to Annex B in sloppy mode.
		// If the initializer is a FunctionLiteral with an Identifier name, predefine the name in current scope
		// so that subsequent references within the same block use the local binding rather than a global.
		if funcLit, ok := declarator.Value.(*parser.FunctionLiteral); ok {
			if funcLit.Name != nil {
				// Predefine to enable using it before assignment in the block
				c.currentSymbolTable.Define(declarator.Name.Value, nilRegister)
			}
		}
		// Set current declarator in legacy fields for backward compatibility
		node.Name = declarator.Name
		node.Value = declarator.Value
		node.ComputedType = declarator.ComputedType

		// Strict mode validation: FutureReservedWords cannot be used as variable names
		if c.chunk.IsStrict && isFutureReservedWord(declarator.Name.Value) {
			c.addError(declarator.Name, fmt.Sprintf("SyntaxError: Unexpected strict mode reserved word '%s'", declarator.Name.Value))
		}

		// debug disabled
		var valueReg Register = nilRegister
		var err errors.PaseratiError
		isValueFunc := false // Flag to track if value is a function literal

		if funcLit, ok := declarator.Value.(*parser.FunctionLiteral); ok {
			isValueFunc = true
			// --- Handle var f = function g() {} or var f = function() {} ---

			// Check if variable was already pre-defined during block var hoisting
			// (this happens when nested functions capture this variable as an upvalue)
			var closureReg Register
			preDefinedReg := nilRegister
			if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister && !sym.IsGlobal {
				// Variable was pre-defined, reuse its register for the closure
				preDefinedReg = sym.Register
				closureReg = preDefinedReg
				debugPrintf("// [VarStmt] Function '%s' was pre-defined in R%d, reusing register\n", node.Name.Value, preDefinedReg)
			} else {
				// 1. Define the *variable name (f)* temporarily for potential recursion
				//    within the function body (e.g., recursive anonymous function).
				c.currentSymbolTable.Define(node.Name.Value, nilRegister)
				// Allocate new register for the closure
				closureReg = c.regAlloc.Alloc()
			}

			// 2. Compile the function literal body.
			//    Pass the variable name (f) as the hint for the function object's name
			//    if the function literal itself is anonymous.
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, node.Name.Value)
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return BadRegister, nil // Return nil error here, main error is tracked
			}
			// 3. Create the closure object in the appropriate register
			c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols)

			// 4. Update the symbol table entry for the *variable name (f)* with the closure register.
			//    (only needed if we didn't pre-define with a register)
			if preDefinedReg == nilRegister {
				c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)
			}

			// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure

			// The variable's value (the closure) is now set.
			// We don't need to assign to valueReg anymore for this path.

		} else if arrowFunc, ok := declarator.Value.(*parser.ArrowFunctionLiteral); ok {
			isValueFunc = true
			// --- Handle var f = () => {} ---

			// Check if variable was already pre-defined during block var hoisting
			var closureReg Register
			preDefinedReg := nilRegister
			if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister && !sym.IsGlobal {
				preDefinedReg = sym.Register
				closureReg = preDefinedReg
			} else {
				c.currentSymbolTable.Define(node.Name.Value, nilRegister)
				closureReg = c.regAlloc.Alloc()
			}

			// Compile the arrow function with variable name as the name hint
			funcConstIndex, freeSymbols, err := c.compileArrowFunctionWithName(arrowFunc, node.Name.Value)
			if err != nil {
				return BadRegister, nil
			}
			// Create a minimal FunctionLiteral for emitClosure
			var body *parser.BlockStatement
			if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
				body = blockBody
			} else {
				body = &parser.BlockStatement{}
			}
			minimalFuncLit := &parser.FunctionLiteral{Body: body}
			c.emitClosure(closureReg, funcConstIndex, minimalFuncLit, freeSymbols)

			if preDefinedReg == nilRegister {
				c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)
			}

		} else if classExpr, ok := declarator.Value.(*parser.ClassExpression); ok {
			// --- Handle var C = class {} or var C = class D {} ---
			// For anonymous class expressions, infer name from variable binding
			// For named class expressions, keep the class's own name
			if classExpr.Name == nil {
				// Anonymous class - set name from variable binding
				classExpr.Name = &parser.Identifier{
					Token: classExpr.Token,
					Value: node.Name.Value,
				}
			}
			// Now compile normally - the class will use its name (either own name or inferred)
			valueReg = c.regAlloc.Alloc()
			_, err = c.compileNode(classExpr, valueReg)
			if err != nil {
				return BadRegister, err
			}

		} else if node.Value != nil {
			// Compile other value types normally
			// DON'T defer free - we'll free explicitly below if needed (when it's a temp, not a variable register)
			valueReg = c.regAlloc.Alloc()
			_, err = c.compileNode(node.Value, valueReg)
			if err != nil {
				return BadRegister, err
			}
		} // else: node.Value is nil (implicit undefined handled below)

		// Handle implicit undefined (`var x;`)
		if valueReg == nilRegister && !isValueFunc {
			// ECMAScript: If the variable is already defined as a PARAMETER,
			// `var x;` should NOT reset it to undefined - it's a no-op for the value.
			// However, for function name bindings (e.g., `function n() { var n; }`),
			// `var n;` should create a new binding initialized to undefined.
			if c.parameterNames != nil && c.parameterNames[node.Name.Value] {
				// Variable is a function parameter - preserve its value
				debugPrintf("// [VarStmt] '%s' is a parameter, skipping undefined init\n", node.Name.Value)
			} else if sym, funcTable := c.findVarInFunctionScope(node.Name.Value); funcTable != nil && sym.Register != nilRegister && !sym.IsGlobal {
				// Variable was already pre-defined during block var hoisting IN THE CURRENT FUNCTION
				// The register already has undefined loaded, so nothing to do
				debugPrintf("// [VarStmt] '%s' was pre-defined in R%d, skipping (already initialized to undefined)\n", node.Name.Value, sym.Register)
			} else if sym, funcTable := c.findVarInFunctionScope(node.Name.Value); funcTable != nil && sym.IsSpilled {
				// Variable was pre-defined as spilled IN THE CURRENT FUNCTION, already initialized to undefined
				debugPrintf("// [VarStmt] '%s' was pre-defined as SPILLED (slot %d), skipping\n", node.Name.Value, sym.SpillIndex)
			} else if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.IsGlobal {
				// ECMAScript: var re-declaration without initializer is a no-op if variable already exists
				// Global variable already exists, skip undefined initialization
				debugPrintf("// [VarStmt] '%s' already exists as global, skipping undefined init\n", node.Name.Value)
			} else {
				// DON'T defer free - we'll track if this becomes a variable register below
				undefReg := c.regAlloc.Alloc()
				c.emitLoadUndefined(undefReg, node.Name.Token.Line)
				valueReg = undefReg
				// Define symbol for the `var x;` case
				// debug disabled
				if c.enclosing == nil {
					// Top-level: use global variable
					globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
					c.emitSetGlobal(globalIdx, valueReg, node.Name.Token.Line)
					c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
					// valueReg was temp for the global, free it
					c.regAlloc.Free(valueReg)
				} else {
					// Function scope: use local symbol table - valueReg becomes the variable's register
					c.currentSymbolTable.Define(node.Name.Value, valueReg)
					// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
					// DON'T free valueReg - it's now the variable's permanent register
				}
			}
		} else if !isValueFunc {
			// Define symbol ONLY for non-function values.
			// Function assignments were handled above by UpdateRegister.
			// Check if variable was already pre-defined during var hoisting (works for both top-level and function scope)
			// Exclude global symbols since they use OpSetGlobal, not local registers
			if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.IsSpilled {
				// Variable was pre-defined as SPILLED, store to spill slot
				debugPrintf("// [VarStmt] Variable '%s' was pre-defined as SPILLED (slot %d), emitting store from R%d\n", node.Name.Value, sym.SpillIndex, valueReg)
				c.emitStoreSpill(sym.SpillIndex, valueReg, node.Name.Token.Line)
				// valueReg was temp, free it
				c.regAlloc.Free(valueReg)
			} else if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister && !sym.IsGlobal {
				// Variable was pre-defined with a local register, reuse it and emit move
				debugPrintf("// [VarStmt] Variable '%s' was pre-defined in R%d, emitting move from R%d\n", node.Name.Value, sym.Register, valueReg)
				c.emitMove(sym.Register, valueReg, node.Name.Token.Line)
				// valueReg was temp, free it
				c.regAlloc.Free(valueReg)
			} else if c.enclosing == nil {
				// Top-level and not pre-defined: use global variable
				globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
				c.emitSetGlobal(globalIdx, valueReg, node.Name.Token.Line)
				c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
				// valueReg was temp for the global, free it
				c.regAlloc.Free(valueReg)
			} else {
				// Function scope and not pre-defined: use local symbol table - valueReg becomes the variable's register
				debugPrintf("// [VarStmt] Defining '%s' in local symbol table with register R%d\n", node.Name.Value, valueReg)
				c.currentSymbolTable.Define(node.Name.Value, valueReg)
				// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
				// DON'T free valueReg - it's now the variable's permanent register
				debugPrintf("// [VarStmt] Successfully defined '%s'\n", node.Name.Value)
			}
		} else if c.enclosing == nil {
			// Top-level function: also set as global
			globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
			// Get the closure register from the symbol table
			symbolRef, _, found := c.currentSymbolTable.Resolve(node.Name.Value)
			if found && symbolRef.Register != nilRegister {
				c.emitSetGlobal(globalIdx, symbolRef.Register, node.Name.Token.Line)
				// Update the symbol to be global
				c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
				// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
			}
		}
	}

	return BadRegister, nil
}

func (c *Compiler) compileConstStatement(node *parser.ConstStatement, hint Register) (Register, errors.PaseratiError) {
	// Process all constant declarations in the statement
	for _, declarator := range node.Declarations {
		// Set current declarator in legacy fields for backward compatibility
		node.Name = declarator.Name
		node.Value = declarator.Value
		node.ComputedType = declarator.ComputedType

		// Strict mode validation: FutureReservedWords cannot be used as variable names
		if c.chunk.IsStrict && isFutureReservedWord(declarator.Name.Value) {
			c.addError(declarator.Name, fmt.Sprintf("SyntaxError: Unexpected strict mode reserved word '%s'", declarator.Name.Value))
		}

		if node.Value == nil {
			// Parser should prevent this, but defensive check
			return BadRegister, NewCompileError(node.Name, "const declarations require an initializer")
		}
		var valueReg Register = nilRegister
		var err errors.PaseratiError
		isValueFunc := false // Flag

		if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
			isValueFunc = true
			// --- Handle const f = function g() {} or const f = function() {} ---
			// 1. Define the *const name (f)* temporarily for recursion.
			c.currentSymbolTable.Define(node.Name.Value, nilRegister)

			// 2. Compile the function literal body, passing const name as hint.
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, node.Name.Value)
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return BadRegister, nil // Return nil error here, main error is tracked
			}
			// 3. Create the closure object
			closureReg := c.regAlloc.Alloc()
			c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols)

			// 4. Update the temporary definition for the *const name (f)* with the closure register.
			c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)

			// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure

			// The constant's value (the closure) is now set.
			// We don't need to assign to valueReg anymore for this path.

		} else if arrowFunc, ok := node.Value.(*parser.ArrowFunctionLiteral); ok {
			isValueFunc = true
			// --- Handle const f = () => {} ---
			// 1. Define the *const name (f)* temporarily for recursion
			c.currentSymbolTable.Define(node.Name.Value, nilRegister)

			// 2. Compile the arrow function with const name as the name hint
			funcConstIndex, freeSymbols, err := c.compileArrowFunctionWithName(arrowFunc, node.Name.Value)
			if err != nil {
				return BadRegister, nil
			}
			// 3. Create the closure object
			closureReg := c.regAlloc.Alloc()
			// Create a minimal FunctionLiteral for emitClosure
			var body *parser.BlockStatement
			if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
				body = blockBody
			} else {
				body = &parser.BlockStatement{}
			}
			minimalFuncLit := &parser.FunctionLiteral{Body: body}
			c.emitClosure(closureReg, funcConstIndex, minimalFuncLit, freeSymbols)

			// 4. Update the symbol table entry for the const with the closure register
			c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)

		} else if classExpr, ok := node.Value.(*parser.ClassExpression); ok {
			// --- Handle const C = class {} or const C = class D {} ---
			// For anonymous class expressions, infer name from variable binding
			// For named class expressions, keep the class's own name
			if classExpr.Name == nil {
				// Anonymous class - set name from variable binding
				classExpr.Name = &parser.Identifier{
					Token: classExpr.Token,
					Value: node.Name.Value,
				}
			}
			// Now compile normally - the class will use its name (either own name or inferred)
			valueReg = c.regAlloc.Alloc()
			_, err = c.compileNode(classExpr, valueReg)
			if err != nil {
				return BadRegister, err
			}

		} else {
			// Compile other value types normally
			// Use existing predefined register if present
			targetReg := valueReg
			if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
				targetReg = sym.Register
			} else {
				// Allocate for the new variable - DON'T defer free since this may become the variable's permanent register
				targetReg = c.regAlloc.Alloc()
			}
			_, err = c.compileNode(node.Value, targetReg)
			if err != nil {
				return BadRegister, err
			}
			valueReg = targetReg
		}

		// Define symbol ONLY for non-function values.
		// Const function assignments were handled above by UpdateRegister.
		if !isValueFunc {
			// For non-functions, Define associates the name with the final value register.
			// Check if we're in global scope: no enclosing compiler AND no outer symbol table
			// For indirect eval, let/const should be local even at top level
			isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil && !c.isIndirectEval
			if isGlobalScope {
				// True global scope: use global variable
				globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
				c.emitSetGlobal(globalIdx, valueReg, node.Name.Token.Line)
				c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
			} else {
				// Local scope (function or enclosed block): use local symbol table
				if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
					c.emitMove(sym.Register, valueReg, node.Name.Token.Line)
				} else {
					// valueReg becomes the variable's permanent register - don't free it
					c.currentSymbolTable.Define(node.Name.Value, valueReg)
					// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
				}
			}
		} else {
			// Function value - check if it should be global
			// For indirect eval, let/const should be local even at top level
			isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil && !c.isIndirectEval
			if isGlobalScope {
				// Top-level function: also set as global
				globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
				// Get the closure register from the symbol table
				symbolRef, _, found := c.currentSymbolTable.Resolve(node.Name.Value)
				if found && symbolRef.Register != nilRegister {
					c.emitSetGlobal(globalIdx, symbolRef.Register, node.Name.Token.Line)
					// Update the symbol to be global
					c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
					// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
				}
			}
		}
		// Mark TDZ as initialized now that the const declaration has been processed
		c.currentSymbolTable.InitializeTDZ(node.Name.Value)
	} // end for declarator
	return BadRegister, nil
}

// isExpressionInTailPosition checks if an expression is in tail position
// Only direct calls, ternary conditionals (with tail branches), and short-circuit operators (with tail RHS) can be in tail position
func (c *Compiler) isExpressionInTailPosition(expr parser.Expression) bool {
	switch e := expr.(type) {
	case *parser.CallExpression:
		return true
	case *parser.TaggedTemplateExpression:
		// Tagged templates are function calls and can be in tail position
		return true
	case *parser.TernaryExpression:
		// Both branches must be in tail position
		return c.isExpressionInTailPosition(e.Consequence) && c.isExpressionInTailPosition(e.Alternative)
	case *parser.InfixExpression:
		// Only for short-circuit operators (&& || ??), and RHS must be in tail position
		if e.Operator == "&&" || e.Operator == "||" || e.Operator == "??" {
			return c.isExpressionInTailPosition(e.Right)
		}
		return false
	default:
		return false
	}
}

func (c *Compiler) compileReturnStatement(node *parser.ReturnStatement, hint Register) (Register, errors.PaseratiError) {
	if node.ReturnValue != nil {
		// Enable tail position only if the return value can be in tail position
		if c.isExpressionInTailPosition(node.ReturnValue) {
			oldTailPos := c.inTailPosition
			c.inTailPosition = true
			defer func() { c.inTailPosition = oldTailPos }()
		}

		var err errors.PaseratiError
		var returnReg Register
		// Check if the return value is a function literal itself
		if funcLit, ok := node.ReturnValue.(*parser.FunctionLiteral); ok {
			// Compile directly, bypassing the compileNode case for declarations.
			// Pass empty hint as it's an anonymous function expression here.
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, "")
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return BadRegister, nil // Return nil error here, main error is tracked
			}
			// Create the closure object
			closureReg := c.regAlloc.Alloc()
			defer c.regAlloc.Free(closureReg)
			c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols)
			returnReg = closureReg

		} else {
			// Compile other expression types normally via compileNode
			returnReg = c.regAlloc.Alloc()
			defer c.regAlloc.Free(returnReg)
			_, err = c.compileNode(node.ReturnValue, returnReg)
			if err != nil {
				return BadRegister, err
			}
		}

		// Error check should cover both paths now
		if err != nil {
			// This check might be redundant if errors are handled correctly above,
			// but keep for safety unless proven otherwise.
			return BadRegister, err
		}
		// Clean up any active iterators before returning from within a loop
		for i := len(c.loopContextStack) - 1; i >= 0; i-- {
			ctx := c.loopContextStack[i]
			if ctx.IteratorCleanup != nil && ctx.IteratorCleanup.UsesIteratorProtocol {
				c.emitIteratorCleanup(ctx.IteratorCleanup.IteratorReg, node.Token.Line)
			}
		}

		// Emit return using the register holding the final value (closure or other expression result)
		// Choose opcode based on whether we're in a finally block or try-with-finally context
		if c.inFinallyBlock || c.tryFinallyDepth > 0 {
			c.emitReturnFinally(returnReg, node.Token.Line)
		} else {
			c.emitReturn(returnReg, node.Token.Line)
		}
		return returnReg, nil // Return the register containing the returned value
	} else {
		// Clean up any active iterators before returning from within a loop
		for i := len(c.loopContextStack) - 1; i >= 0; i-- {
			ctx := c.loopContextStack[i]
			if ctx.IteratorCleanup != nil && ctx.IteratorCleanup.UsesIteratorProtocol {
				c.emitIteratorCleanup(ctx.IteratorCleanup.IteratorReg, node.Token.Line)
			}
		}

		// Return undefined implicitly using the optimized opcode
		if c.inFinallyBlock || c.tryFinallyDepth > 0 {
			// For finally blocks or try-with-finally contexts, we need to use OpReturnFinally even for undefined returns
			// Allocate a temporary register for undefined
			undefinedReg := c.regAlloc.Alloc()
			defer c.regAlloc.Free(undefinedReg)
			c.emitOpCode(vm.OpLoadUndefined, node.Token.Line)
			c.emitByte(byte(undefinedReg))
			c.emitReturnFinally(undefinedReg, node.Token.Line)
		} else {
			c.emitOpCode(vm.OpReturnUndefined, node.Token.Line)
		}
		// For undefined returns, we could allocate a register with undefined, but for now return BadRegister
		return BadRegister, nil
	}
}

// --- Loop Compilation (Updated) ---

func (c *Compiler) compileWhileStatement(node *parser.WhileStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileWhileStatementLabeled(node, "", hint)
}

func (c *Compiler) compileWhileStatementLabeled(node *parser.WhileStatement, label string, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// Defer-safety: patch any outstanding placeholder jump to a valid anchor on early returns
	var (
		jumpToEndPlaceholderPos = -1
		patchedCondition        = false
	)
	defer func() {
		if jumpToEndPlaceholderPos >= 0 && !patchedCondition {
			c.patchJump(jumpToEndPlaceholderPos)
			patchedCondition = true
		}
	}()

	// Per ECMAScript spec, initialize completion value V = undefined
	c.emitLoadUndefined(hint, line)

	// --- Setup Loop Context ---
	loopStartPos := len(c.chunk.Code) // Position before condition evaluation
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos,
		ContinueTargetPos:          loopStartPos, // Continue goes back to condition in while
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
		CompletionReg:              hint, // For break/continue to update via UpdateEmpty
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// --- Compile Condition ---
	conditionReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(conditionReg)
	_, err := c.compileNode(node.Condition, conditionReg)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1] // Pop context on error
		return BadRegister, NewCompileError(node, "error compiling while condition").CausedBy(err)
	}

	// --- Jump Out If False ---
	jumpToEndPlaceholderPos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, line)

	// --- Compile Body ---
	// Per ECMAScript spec, if body produces a value, update V (the completion value in hint)
	// Compile directly with hint so break/continue inside nested try-finally can access
	// the completion value correctly
	_, err = c.compileNode(node.Body, hint)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1] // Pop context on error
		return BadRegister, NewCompileError(node, "error compiling while body").CausedBy(err)
	}

	// --- Jump Back To Start ---
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2 // OpCode + 16bit offset
	backOffset := loopStartPos - jumpBackInstructionEndPos
	c.emitOpCode(vm.OpJump, line)
	c.emitUint16(uint16(int16(backOffset))) // Emit calculated signed offset

	// --- Finish Loop ---
	// Patch the initial conditional jump to land here (after the backward jump)
	c.patchJump(jumpToEndPlaceholderPos)
	patchedCondition = true

	// Pop context and patch breaks
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
	for _, breakPlaceholderPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPlaceholderPos) // Patch break jumps to loop end
	}

	// --- NEW: Patch continue jumps ---
	// Continue jumps back to the condition check (loopStartPos)
	for _, continuePos := range poppedContext.ContinuePlaceholderPosList {
		jumpInstructionEndPos := continuePos + 1 + 2 // OpCode + 16bit offset
		targetOffset := poppedContext.LoopStartPos - jumpInstructionEndPos

		if targetOffset > math.MaxInt16 || targetOffset < math.MinInt16 {
			return BadRegister, NewCompileError(node, fmt.Sprintf("internal compiler error: continue jump offset %d exceeds 16-bit limit", targetOffset))
		}
		// Manually write the 16-bit offset into the placeholder jump instruction
		c.chunk.Code[continuePos+1] = byte(int16(targetOffset) >> 8)   // High byte
		c.chunk.Code[continuePos+2] = byte(int16(targetOffset) & 0xFF) // Low byte
	}

	// Return completion value (which is undefined if loop never ran, or last body value)
	return hint, nil
}

func (c *Compiler) compileForStatement(node *parser.ForStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileForStatementLabeled(node, "", hint)
}

func (c *Compiler) compileForStatementLabeled(node *parser.ForStatement, label string, hint Register) (Register, errors.PaseratiError) {
	// Defer-safety: patch any outstanding placeholder jump to a valid anchor on early returns
	var (
		conditionExitJumpPlaceholderPos = -1
		patchedConditionExit            = false
	)
	defer func() {
		if conditionExitJumpPlaceholderPos != -1 && !patchedConditionExit {
			c.patchJump(conditionExitJumpPlaceholderPos)
			patchedConditionExit = true
		}
	}()
	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Per ECMAScript spec, for loops with let/const declarations create a new lexical scope
	// that encompasses the entire loop (initializer, condition, update, and body).
	// This scope is separate from the outer scope, so variables declared with let/const
	// in the initializer shadow outer variables but don't modify them.
	var prevSymbolTable *SymbolTable
	hasLexicalDecl := false
	if node.Initializer != nil {
		_, isLet := node.Initializer.(*parser.LetStatement)
		_, isConst := node.Initializer.(*parser.ConstStatement)
		// For destructuring declarations, check if it's let/const (not var)
		if arrDestr, ok := node.Initializer.(*parser.ArrayDestructuringDeclaration); ok {
			hasLexicalDecl = arrDestr.Token.Type == lexer.LET || arrDestr.Token.Type == lexer.CONST
		} else if objDestr, ok := node.Initializer.(*parser.ObjectDestructuringDeclaration); ok {
			hasLexicalDecl = objDestr.Token.Type == lexer.LET || objDestr.Token.Type == lexer.CONST
		} else {
			hasLexicalDecl = isLet || isConst
		}
	}
	// Track per-iteration binding registers for let/const in for loop init.
	// ECMAScript spec: each iteration gets fresh bindings, so closures capture
	// different values per iteration. We close upvalues at end of each iteration.
	var perIterationRegs []Register
	if hasLexicalDecl {
		prevSymbolTable = c.currentSymbolTable
		c.currentSymbolTable = NewEnclosedSymbolTable(c.currentSymbolTable)
	}
	// Ensure we restore the scope when done
	defer func() {
		if prevSymbolTable != nil {
			c.currentSymbolTable = prevSymbolTable
		}
	}()

	// 1. Initializer
	if node.Initializer != nil {
		// If initializer is a var/let/const declaration, predefine its bindings so Resolve() works in condition/update.
		if vs, ok := node.Initializer.(*parser.VarStatement); ok {
			// Prefer explicit declarations if present; otherwise fall back to legacy Name field
			if len(vs.Declarations) > 0 {
				for _, d := range vs.Declarations {
					isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil
					if isGlobalScope {
						// True global scope: predefine as global so identifier resolves as global in condition/update
						globalIdx := c.GetOrAssignGlobalIndex(d.Name.Value)
						c.currentSymbolTable.DefineGlobal(d.Name.Value, globalIdx)
					} else {
						// Local scope: predefine local binding; register will be set by compileVarStatement
						c.currentSymbolTable.Define(d.Name.Value, nilRegister)
					}
				}
			} else if vs.Name != nil {
				name := vs.Name.Value
				isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil
				if isGlobalScope {
					globalIdx := c.GetOrAssignGlobalIndex(name)
					c.currentSymbolTable.DefineGlobal(name, globalIdx)
				} else {
					c.currentSymbolTable.Define(name, nilRegister)
				}
			}
		} else if ls, ok := node.Initializer.(*parser.LetStatement); ok {
			// Handle let declarations in for loop
			// Pre-allocate registers and pin them so closures can capture these variables
			// Track registers for per-iteration bindings
			if len(ls.Declarations) > 0 {
				for _, d := range ls.Declarations {
					isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil
					if isGlobalScope {
						globalIdx := c.GetOrAssignGlobalIndex(d.Name.Value)
						c.currentSymbolTable.DefineGlobal(d.Name.Value, globalIdx)
					} else {
						reg, ok := c.regAlloc.TryAllocForVariable()
						if ok {
							c.currentSymbolTable.Define(d.Name.Value, reg)
							c.regAlloc.Pin(reg) // Pin so closures can capture
							perIterationRegs = append(perIterationRegs, reg)
						} else {
							// Spill if no registers available
							spillIdx := c.AllocSpillSlot()
							c.currentSymbolTable.DefineSpilled(d.Name.Value, spillIdx)
							// Note: spilled vars don't need OpCloseUpvalue - they're not captured as upvalues
						}
					}
				}
			} else if ls.Name != nil {
				name := ls.Name.Value
				isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil
				if isGlobalScope {
					globalIdx := c.GetOrAssignGlobalIndex(name)
					c.currentSymbolTable.DefineGlobal(name, globalIdx)
				} else {
					reg, ok := c.regAlloc.TryAllocForVariable()
					if ok {
						c.currentSymbolTable.Define(name, reg)
						c.regAlloc.Pin(reg)
						perIterationRegs = append(perIterationRegs, reg)
					} else {
						spillIdx := c.AllocSpillSlot()
						c.currentSymbolTable.DefineSpilled(name, spillIdx)
					}
				}
			}
		} else if cs, ok := node.Initializer.(*parser.ConstStatement); ok {
			// Handle const declarations in for loop
			// Pre-allocate registers and pin them so closures can capture these variables
			// Track registers for per-iteration bindings
			// NOTE: Use DefineConstTDZ to mark as const so assignment throws TypeError
			if len(cs.Declarations) > 0 {
				for _, d := range cs.Declarations {
					isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil
					if isGlobalScope {
						globalIdx := c.GetOrAssignGlobalIndex(d.Name.Value)
						c.currentSymbolTable.DefineGlobal(d.Name.Value, globalIdx)
						// TODO: Need DefineGlobalConst to mark global const
					} else {
						reg, ok := c.regAlloc.TryAllocForVariable()
						if ok {
							c.currentSymbolTable.DefineConstTDZ(d.Name.Value, reg)
							c.regAlloc.Pin(reg)
							perIterationRegs = append(perIterationRegs, reg)
						} else {
							spillIdx := c.AllocSpillSlot()
							c.currentSymbolTable.DefineConstTDZSpilled(d.Name.Value, spillIdx)
						}
					}
				}
			} else if cs.Name != nil {
				name := cs.Name.Value
				isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil
				if isGlobalScope {
					globalIdx := c.GetOrAssignGlobalIndex(name)
					c.currentSymbolTable.DefineGlobal(name, globalIdx)
					// TODO: Need DefineGlobalConst to mark global const
				} else {
					reg, ok := c.regAlloc.TryAllocForVariable()
					if ok {
						c.currentSymbolTable.DefineConstTDZ(name, reg)
						c.regAlloc.Pin(reg)
						perIterationRegs = append(perIterationRegs, reg)
					} else {
						spillIdx := c.AllocSpillSlot()
						c.currentSymbolTable.DefineConstTDZSpilled(name, spillIdx)
					}
				}
			}
		}
		initReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, initReg)
		if _, err := c.compileNode(node.Initializer, initReg); err != nil {
			return BadRegister, err
		}
	}

	// *** Per-iteration bindings: close upvalues AFTER init but BEFORE first iteration ***
	// ECMAScript spec 13.7.4.8 ForBodyEvaluation:
	//   Step 2: "Perform ? CreatePerIterationEnvironment(perIterationBindings)"
	// This must happen BEFORE the first test, so closures created in init capture
	// the initial values, separate from closures created in test/body/update.
	for _, reg := range perIterationRegs {
		c.emitCloseUpvalue(reg, node.Token.Line)
	}

	// Per ECMAScript spec, initialize completion value V = undefined
	c.emitLoadUndefined(hint, node.Token.Line)

	// --- Loop Start & Context Setup ---
	loopStartPos := len(c.chunk.Code) // Position before condition check
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
		CompletionReg:              hint, // For break/continue to update via UpdateEmpty
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)
	// Scope for body/vars is handled by compileNode for the BlockStatement

	// --- 2. Condition (Optional) ---
	if node.Condition != nil {
		conditionReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, conditionReg)
		_, err := c.compileNode(node.Condition, conditionReg)
		if err != nil {
			// Clean up loop context if condition compilation fails
			c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
			return BadRegister, err
		}
		conditionExitJumpPlaceholderPos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)
	} // If no condition, it's an infinite loop (handled by break/return)

	// --- 3. Body ---
	// Continue placeholders will be added to loopContext here
	// Per ECMAScript spec, if body produces a value, update V (the completion value in hint)
	// Compile directly with hint so break/continue inside nested try-finally can access
	// the completion value correctly
	_, err := c.compileNode(node.Body, hint)
	if err != nil {
		// Clean up loop context if body compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, err
	}

	// --- 4. Patch Continues & Compile Update ---

	// *** Patch Continue Jumps ***
	// Patch continue jumps to land here, *before* the update expression
	for _, continuePos := range loopContext.ContinuePlaceholderPosList { // Use context on stack
		c.patchJump(continuePos) // Patch placeholder to jump to current position
	}

	// *** Per-iteration bindings: close upvalues for let/const loop variables ***
	// ECMAScript spec: each iteration gets fresh bindings. This is achieved by
	// closing any upvalues pointing to these registers, so closures capture
	// the value at this iteration. The register keeps its value for the update.
	for _, reg := range perIterationRegs {
		c.emitCloseUpvalue(reg, node.Token.Line)
	}

	// *** Compile Update Expression (Optional) ***
	if node.Update != nil {
		updateReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, updateReg)
		if _, err := c.compileNode(node.Update, updateReg); err != nil {
			// Clean up loop context if update compilation fails
			c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
			return BadRegister, err
		}
		// Result of update expression is discarded implicitly by not using c.lastReg
	}

	// --- 5. Jump back to Loop Start (before condition) ---
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2 // OpCode + 16bit offset
	backOffset := loopStartPos - jumpBackInstructionEndPos
	c.emitOpCode(vm.OpJump, node.Body.Token.Line) // Use body's line for jump back
	c.emitUint16(uint16(int16(backOffset)))

	// --- 6. Loop End & Patch Condition/Breaks ---

	// Position *after* the loop (target for breaks/condition exit) is implicitly len(c.chunk.Code)

	// Pop loop context
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	// Patch the condition exit jump (if there was a condition)
	// This needs to happen *at* the final position
	if conditionExitJumpPlaceholderPos != -1 {
		c.patchJump(conditionExitJumpPlaceholderPos) // Patch to jump to current position
		patchedConditionExit = true
	}

	// Patch all break jumps
	// This needs to happen *at* the final position
	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos) // Patch to jump to current position
	}

	// Return completion value
	return hint, nil
}

// --- New: Break/Continue Compilation ---

func (c *Compiler) compileBreakStatement(node *parser.BreakStatement, hint Register) (Register, errors.PaseratiError) {
	if len(c.loopContextStack) == 0 {
		return BadRegister, NewCompileError(node, "break statement not within a loop")
	}

	var targetContext *LoopContext

	if node.Label != nil {
		// Find the labeled context
		found := false
		for i := len(c.loopContextStack) - 1; i >= 0; i-- {
			if c.loopContextStack[i].Label == node.Label.Value {
				targetContext = c.loopContextStack[i]
				found = true
				break
			}
		}
		if !found {
			return BadRegister, NewCompileError(node, fmt.Sprintf("label '%s' not found", node.Label.Value))
		}
	} else {
		// Get current loop context (top of stack)
		targetContext = c.loopContextStack[len(c.loopContextStack)-1]
	}

	// Check if we need to emit iterator cleanup code before breaking
	if targetContext.IteratorCleanup != nil && targetContext.IteratorCleanup.UsesIteratorProtocol {
		c.emitIteratorCleanup(targetContext.IteratorCleanup.IteratorReg, node.Token.Line)
	}

	// Per ECMAScript spec, break has an empty completion value.
	// UpdateEmpty(break, V) keeps V if break's value is empty.
	// When break is inside nested structures (like if statements), the current
	// completion value (in hint) may be in a different register than the loop's
	// completion register. We need to copy it before jumping.
	if targetContext.CompletionReg != BadRegister && hint != BadRegister && hint != targetContext.CompletionReg {
		// Copy the current completion value to the loop's completion register
		// This handles cases like: do { if (true) { 3; break; } } while (false)
		// where 3 is in the if's hint register but needs to be in the loop's completion register
		c.emitMove(targetContext.CompletionReg, hint, node.Token.Line)
	}

	// Check if we're inside a try-finally block AND the break targets a loop outside it
	// BUT NOT if we're already inside the finally block itself - in that case, just emit direct jump
	if len(c.finallyContextStack) > 0 && !c.inFinallyBlock {
		finallyCtx := c.finallyContextStack[len(c.finallyContextStack)-1]

		// Find the index of the target loop in the stack
		targetLoopIndex := -1
		for i, ctx := range c.loopContextStack {
			if ctx == targetContext {
				targetLoopIndex = i
				break
			}
		}

		// Only jump to finally if the target loop was on the stack BEFORE the finally context was created
		// This means the break would exit the try-finally block
		if targetLoopIndex >= 0 && targetLoopIndex < finallyCtx.LoopStackDepthAtCreation {
			// Break targets a loop outside try-finally: push completion and jump to finally
			opcodePos := len(c.chunk.Code)
			c.emitOpCode(vm.OpPushBreak, node.Token.Line)
			c.emitUint16(0xFFFF) // Placeholder for target PC offset

			// Add opcode position to loop's break list (will be patched when loop finishes)
			targetContext.BreakPlaceholderPosList = append(targetContext.BreakPlaceholderPosList, opcodePos)

			// Emit jump to finally block
			jumpPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
			finallyCtx.JumpToFinallyPlaceholders = append(finallyCtx.JumpToFinallyPlaceholders, jumpPos)
		} else {
			// Break targets a loop inside try block: emit normal break jump
			placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
			targetContext.BreakPlaceholderPosList = append(targetContext.BreakPlaceholderPosList, placeholderPos)
		}
	} else {
		// Not in try-finally: emit normal break jump
		placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
		targetContext.BreakPlaceholderPosList = append(targetContext.BreakPlaceholderPosList, placeholderPos)
	}

	return BadRegister, nil
}

func (c *Compiler) compileEmptyStatement(node *parser.EmptyStatement, hint Register) (Register, errors.PaseratiError) {
	// Empty statements are no-ops - they generate no bytecode
	// This is perfectly valid: if (condition) ; else doSomething();
	return hint, nil
}

func (c *Compiler) compileContinueStatement(node *parser.ContinueStatement, hint Register) (Register, errors.PaseratiError) {
	if len(c.loopContextStack) == 0 {
		return BadRegister, NewCompileError(node, "continue statement not within a loop")
	}

	var targetContext *LoopContext

	if node.Label != nil {
		// Find the labeled context
		found := false
		targetIndex := -1
		for i := len(c.loopContextStack) - 1; i >= 0; i-- {
			if c.loopContextStack[i].Label == node.Label.Value {
				targetContext = c.loopContextStack[i]
				targetIndex = i
				found = true
				break
			}
		}
		if !found {
			return BadRegister, NewCompileError(node, fmt.Sprintf("label '%s' not found", node.Label.Value))
		}

		// Check that the target is actually a loop (continue only works with loops)
		if targetContext.ContinueTargetPos == -1 {
			return BadRegister, NewCompileError(node, fmt.Sprintf("continue statement cannot target non-loop label '%s'", node.Label.Value))
		}

		// If continuing to an outer loop, need to close any inner loop iterators
		// Walk from current context backwards to target context
		for i := len(c.loopContextStack) - 1; i > targetIndex; i-- {
			ctx := c.loopContextStack[i]
			if ctx.IteratorCleanup != nil && ctx.IteratorCleanup.UsesIteratorProtocol {
				c.emitIteratorCleanup(ctx.IteratorCleanup.IteratorReg, node.Token.Line)
			}
		}
	} else {
		// Find the nearest enclosing loop context that supports continue
		// (switch statements push contexts with ContinueTargetPos = -1)
		for i := len(c.loopContextStack) - 1; i >= 0; i-- {
			ctx := c.loopContextStack[i]
			if ctx.ContinueTargetPos != -1 {
				targetContext = ctx
				break
			}
		}
		if targetContext == nil {
			return BadRegister, NewCompileError(node, "continue statement not within a loop")
		}
	}

	// Per ECMAScript spec, continue has an empty completion value.
	// UpdateEmpty(continue, V) keeps V if continue's value is empty.
	// When continue is inside nested structures (like if statements), the current
	// completion value (in hint) may be in a different register than the loop's
	// completion register. We need to copy it before jumping.
	if targetContext.CompletionReg != BadRegister && hint != BadRegister && hint != targetContext.CompletionReg {
		// Copy the current completion value to the loop's completion register
		// This handles cases like: do { if (true) { 10; continue; } } while (false)
		// where 10 is in the if's hint register but needs to be in the loop's completion register
		c.emitMove(targetContext.CompletionReg, hint, node.Token.Line)
	}

	// Check if we're inside a try-finally block
	// BUT NOT if we're already inside the finally block itself - in that case, just emit direct jump
	if len(c.finallyContextStack) > 0 && !c.inFinallyBlock {
		finallyCtx := c.finallyContextStack[len(c.finallyContextStack)-1]

		// Find which loop index the target context is at
		targetLoopIndex := -1
		for i, ctx := range c.loopContextStack {
			if ctx == targetContext {
				targetLoopIndex = i
				break
			}
		}

		// Only route through finally if the target loop existed BEFORE the try-finally context
		// This means the continue exits the try-finally block
		if targetLoopIndex >= 0 && targetLoopIndex < finallyCtx.LoopStackDepthAtCreation {
			// Continue exits try-finally: push completion and jump to finally
			// Emit: OpPushContinue <placeholder>
			opcodePos := len(c.chunk.Code)
			c.emitOpCode(vm.OpPushContinue, node.Token.Line)
			c.emitUint16(0xFFFF) // Placeholder for target PC offset

			// Add opcode position to loop's continue list (will be patched when loop finishes)
			targetContext.ContinuePlaceholderPosList = append(targetContext.ContinuePlaceholderPosList, opcodePos)

			// Emit jump to finally block
			jumpPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
			finallyCtx.JumpToFinallyPlaceholders = append(finallyCtx.JumpToFinallyPlaceholders, jumpPos)
		} else {
			// Continue is for a loop inside try block, doesn't exit try-finally
			// Emit normal continue jump (no finally routing needed)
			placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
			targetContext.ContinuePlaceholderPosList = append(targetContext.ContinuePlaceholderPosList, placeholderPos)
		}
	} else {
		// Not in try-finally: emit normal continue jump
		placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
		targetContext.ContinuePlaceholderPosList = append(targetContext.ContinuePlaceholderPosList, placeholderPos)
	}

	return BadRegister, nil
}

// --- New: Labeled Statement Compilation ---

func (c *Compiler) compileLabeledStatement(node *parser.LabeledStatement, hint Register) (Register, errors.PaseratiError) {
	// Compile the labeled statement
	// If it's a loop-like statement (while, for, do-while), we need to set the label in the LoopContext

	// For loop statements, we need to temporarily add a label context
	// and then call the normal compilation functions
	switch stmt := node.Statement.(type) {
	case *parser.WhileStatement:
		return c.compileWhileStatementLabeled(stmt, node.Label.Value, hint)
	case *parser.ForStatement:
		return c.compileForStatementLabeled(stmt, node.Label.Value, hint)
	case *parser.DoWhileStatement:
		return c.compileDoWhileStatementLabeled(stmt, node.Label.Value, hint)
	case *parser.ForOfStatement:
		return c.compileForOfStatementLabeled(stmt, node.Label.Value, hint)
	case *parser.ForInStatement:
		return c.compileForInStatementLabeled(stmt, node.Label.Value, hint)
	default:
		// For non-loop statements, we need to create a label context anyway
		// in case there are break statements that refer to this label
		labelContext := &LoopContext{
			Label:                      node.Label.Value,
			LoopStartPos:               -1, // Not applicable for non-loops
			ContinueTargetPos:          -1, // Not applicable for non-loops
			BreakPlaceholderPosList:    make([]int, 0),
			ContinuePlaceholderPosList: make([]int, 0),
			CompletionReg:              BadRegister, // Non-loops don't track completion values
		}
		c.loopContextStack = append(c.loopContextStack, labelContext)

		// Compile the statement
		result, err := c.compileNode(node.Statement, hint)

		// Pop the label context and patch any break statements
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		for _, placeholderPos := range labelContext.BreakPlaceholderPosList {
			c.patchJump(placeholderPos)
		}

		return result, err
	}
}

func (c *Compiler) compileDoWhileStatement(node *parser.DoWhileStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileDoWhileStatementLabeled(node, "", hint)
}

func (c *Compiler) compileDoWhileStatementLabeled(node *parser.DoWhileStatement, label string, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Per ECMAScript spec, initialize completion value V = undefined
	c.emitLoadUndefined(hint, line)

	// 1. Mark Loop Start (before body)
	loopStartPos := len(c.chunk.Code)

	// 2. Setup Loop Context
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos, // Continue jumps here
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
		CompletionReg:              hint, // For break/continue to update via UpdateEmpty
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// 3. Compile Body (executes at least once)
	// Per ECMAScript spec, if body produces a value, update V (the completion value in hint)
	// Compile directly with hint so break/continue inside nested try-finally can access
	// the completion value correctly
	_, err := c.compileNode(node.Body, hint)
	if err != nil {
		// Pop context if body compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, NewCompileError(node, "error compiling do-while body").CausedBy(err)
	}

	// 3.5. Patch continue jumps to land here (after body, before condition check)
	// For do-while, continue should jump to the condition check, not back to body start
	for _, continuePos := range loopContext.ContinuePlaceholderPosList {
		c.patchJump(continuePos)
	}

	// 4. Mark Condition Position (for clarity, not used directly in jump calcs below)
	_ = len(c.chunk.Code) // conditionPos := len(c.chunk.Code)

	// 5. Compile Condition
	conditionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, conditionReg)
	_, err = c.compileNode(node.Condition, conditionReg)
	if err != nil {
		// Pop context if condition compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, NewCompileError(node, "error compiling do-while condition").CausedBy(err)
	}

	// 6. Conditional Jump back to Loop Start
	// We need OpJumpIfTrue, but we only have OpJumpIfFalse.
	// So, we invert the condition and use OpJumpIfFalse.
	invertedConditionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, invertedConditionReg)
	c.emitNot(invertedConditionReg, conditionReg, line)

	// Now jump back if the *inverted* condition is FALSE (i.e., original was TRUE)
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2 + 1 // OpCode + Reg + 16bit offset
	backOffset := loopStartPos - jumpBackInstructionEndPos
	if backOffset > math.MaxInt16 || backOffset < math.MinInt16 {
		return BadRegister, NewCompileError(node, fmt.Sprintf("internal compiler error: do-while loop jump offset %d exceeds 16-bit limit", backOffset))
	}
	c.emitOpCode(vm.OpJumpIfFalse, line)    // Use OpJumpIfFalse on inverted result
	c.emitByte(byte(invertedConditionReg))  // Jump based on the inverted condition
	c.emitUint16(uint16(int16(backOffset))) // Emit calculated signed offset

	// --- 7. Loop End & Patching ---
	// Position after the loop (target for breaks) is implicitly len(c.chunk.Code)

	// 8. Pop loop context
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	// 9. Patch Break Jumps
	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos) // Patch break jumps to loop end
	}

	// 10. Continue jumps already patched (after body, before condition) in step 3.5

	// Return completion value
	return hint, nil
}

// compileSwitchStatement compiles a switch statement.
//
//	switch (expr) {
//	  case val1: body1; break;
//	  case val2: body2;
//	  default: bodyD; break;
//	}
//
// Compilation strategy:
//  1. Compile switch expression.
//  2. For each case:
//     a. Compile case value.
//     b. Compare with switch expression value (StrictEqual).
//     c. If not equal, jump to the *next* case test (OpJumpIfFalse).
//     d. If equal, execute the case body.
//     e. Handle break: Jumps to the end of the entire switch.
//     f. Implicit fallthrough means after a body (without break), execution continues to the next case test.
//  3. Handle default: If reached (all cases failed), execute default body.
//  4. Patch all jumps.
func (c *Compiler) compileSwitchStatement(node *parser.SwitchStatement, hint Register) (Register, errors.PaseratiError) {
	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the expression being switched on
	switchExprReg := c.regAlloc.Alloc()
	// Note: switchExprReg is freed after all comparisons, not via tempRegs
	_, err := c.compileNode(node.Expression, switchExprReg)
	if err != nil {
		c.regAlloc.Free(switchExprReg)
		return BadRegister, err
	}

	// Find the default case (if any)
	defaultCaseIndex := -1
	for i, caseClause := range node.Cases {
		if caseClause.Condition == nil {
			if defaultCaseIndex != -1 {
				c.addError(node, "Multiple default cases in switch statement")
				return BadRegister, nil
			}
			defaultCaseIndex = i
		}
	}

	// Push a context to handle break statements within the switch
	c.pushLoopContext(-1, -1)

	// Track completion value
	completionReg := hint
	if completionReg == BadRegister {
		completionReg = c.regAlloc.Alloc()
		tempRegs = append(tempRegs, completionReg)
	}
	c.emitLoadUndefined(completionReg, 0)

	// Set the completion register on the loop context so break statements use the correct register
	if loopCtx := c.currentLoopContext(); loopCtx != nil {
		loopCtx.CompletionReg = completionReg
	}

	// === Create lexical scope for switch CaseBlock ===
	// Per ECMAScript spec, the switch CaseBlock creates a single lexical environment
	// that all case clauses share. IMPORTANT: This scope must be created BEFORE
	// evaluating case selectors, so that let/const declarations are hoisted and
	// visible in case selector expressions.
	prevSymbolTable := c.currentSymbolTable
	c.currentSymbolTable = NewEnclosedSymbolTable(prevSymbolTable)

	// Pre-define all let/const declarations found in case bodies (hoisting)
	for _, caseClause := range node.Cases {
		if caseClause.Body != nil {
			for _, stmt := range caseClause.Body.Statements {
				switch s := stmt.(type) {
				case *parser.LetStatement:
					if s.Name != nil {
						if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
							reg, ok := c.regAlloc.TryAllocForVariable()
							if ok {
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							} else {
								spillIdx := c.AllocSpillSlot()
								c.currentSymbolTable.DefineSpilled(s.Name.Value, spillIdx)
							}
						}
					}
				case *parser.ConstStatement:
					if s.Name != nil {
						if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
							reg, ok := c.regAlloc.TryAllocForVariable()
							if ok {
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							} else {
								spillIdx := c.AllocSpillSlot()
								c.currentSymbolTable.DefineSpilled(s.Name.Value, spillIdx)
							}
						}
					}
				}
			}
		}
	}

	// === PHASE 1: Emit all case comparisons ===
	// Each comparison jumps to the corresponding body if matched.
	// If no match, falls through to the next comparison.
	// After all comparisons, jump to default body (if exists) or end.
	jumpToBodyPlaceholders := make([]int, len(node.Cases))

	for i, caseClause := range node.Cases {
		if caseClause.Condition == nil {
			// Default case: no comparison, will be handled after all comparisons
			continue
		}

		caseLine := caseClause.Token.Line

		// Compile the case condition
		caseCondReg := c.regAlloc.Alloc()
		_, err := c.compileNode(caseClause.Condition, caseCondReg)
		if err != nil {
			c.regAlloc.Free(caseCondReg)
			return BadRegister, err
		}

		// Compare switch expression value with case condition value
		matchReg := c.regAlloc.Alloc()
		c.emitStrictEqual(matchReg, switchExprReg, caseCondReg, caseLine)

		// Free caseCondReg - no longer needed after comparison
		c.regAlloc.Free(caseCondReg)

		// Pattern: JumpIfFalse to skip the jump-to-body, then unconditional jump to body
		// JumpIfFalse matchReg, +3 (skip the OpJump instruction which is 3 bytes)
		c.emitOpCode(vm.OpJumpIfFalse, caseLine)
		c.emitByte(byte(matchReg))
		c.emitUint16(3) // Skip the following OpJump (1 byte opcode + 2 bytes offset)

		// Free matchReg - no longer needed after the jump condition
		c.regAlloc.Free(matchReg)

		// If match, jump to the body (placeholder to be patched)
		jumpToBodyPlaceholders[i] = c.emitPlaceholderJump(vm.OpJump, 0, caseLine)
		// If no match (JumpIfFalse taken), continue to the next comparison
	}

	// After all comparisons fail, jump to default body or end
	var jumpToDefaultOrEnd int
	if defaultCaseIndex >= 0 {
		// Jump to default body (will be patched later)
		jumpToDefaultOrEnd = c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
	} else {
		// No default, jump to end (will be patched later)
		jumpToDefaultOrEnd = c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
	}

	// Free switchExprReg - no longer needed after all comparisons
	c.regAlloc.Free(switchExprReg)

	// === PHASE 2: Emit all case bodies in order ===
	// Bodies are emitted in order for natural fall-through.
	// Break statements emit jumps to the end.
	caseBodyPositions := make([]int, len(node.Cases))

	for i, caseClause := range node.Cases {
		caseLine := caseClause.Token.Line

		// Record the body position
		caseBodyPositions[i] = c.currentPosition()

		// Patch the jump from the comparison to this body
		if caseClause.Condition != nil && jumpToBodyPlaceholders[i] != 0 {
			c.patchJump(jumpToBodyPlaceholders[i])
		}

		// Compile the case body statements
		if caseClause.Body != nil && len(caseClause.Body.Statements) > 0 {
			for _, stmt := range caseClause.Body.Statements {
				stmtReg, err := c.compileNode(stmt, completionReg)
				if err != nil {
					return BadRegister, err
				}
				if stmtReg != BadRegister && stmtReg != completionReg {
					c.emitMove(completionReg, stmtReg, caseLine)
				}
			}
		}
		// Fall through to the next case body (no jump emitted)
	}

	// Patch the jump to default body or end
	if defaultCaseIndex >= 0 {
		// Patch to jump to default body position
		// We need to manually patch since the default body was already emitted
		jumpInstructionEndPos := jumpToDefaultOrEnd + 1 + 2
		targetOffset := caseBodyPositions[defaultCaseIndex] - jumpInstructionEndPos
		c.chunk.Code[jumpToDefaultOrEnd+1] = byte(int16(targetOffset) >> 8)
		c.chunk.Code[jumpToDefaultOrEnd+2] = byte(int16(targetOffset) & 0xFF)
	} else {
		// Patch to jump to end (current position)
		c.patchJump(jumpToDefaultOrEnd)
	}

	// Patch all break jumps to point here (end of switch)
	loopCtx := c.currentLoopContext()
	if loopCtx != nil {
		for _, breakJumpPos := range loopCtx.BreakPlaceholderPosList {
			c.patchJump(breakJumpPos)
		}
	}

	// Restore the previous scope
	c.currentSymbolTable = prevSymbolTable

	c.popLoopContext()

	return completionReg, nil
}

func (c *Compiler) compileForOfStatement(node *parser.ForOfStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileForOfStatementLabeled(node, "", hint)
}

func (c *Compiler) compileForInStatement(node *parser.ForInStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileForInStatementLabeled(node, "", hint)
}

func (c *Compiler) compileForInStatementLabeled(node *parser.ForInStatement, label string, hint Register) (Register, errors.PaseratiError) {
	// Defer-safety: ensure condition exit is patched
	var (
		conditionExitJumpPos = -1
		patchedConditionExit = false
	)
	defer func() {
		if conditionExitJumpPos >= 0 && !patchedConditionExit {
			c.patchJump(conditionExitJumpPos)
			patchedConditionExit = true
		}
	}()
	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Check if this is a lexical binding (let/const) that needs its own scope
	// This ensures loop variables shadow outer variables with the same name
	var hasLexicalDecl bool
	var prevSymbolTable *SymbolTable
	switch v := node.Variable.(type) {
	case *parser.LetStatement, *parser.ConstStatement:
		hasLexicalDecl = true
	case *parser.ArrayDestructuringDeclaration:
		hasLexicalDecl = v.Token.Type == lexer.LET || v.Token.Type == lexer.CONST
	case *parser.ObjectDestructuringDeclaration:
		hasLexicalDecl = v.Token.Type == lexer.LET || v.Token.Type == lexer.CONST
	}
	if hasLexicalDecl {
		prevSymbolTable = c.currentSymbolTable
		c.currentSymbolTable = NewEnclosedSymbolTable(c.currentSymbolTable)
	}
	// Ensure we restore the scope when done
	defer func() {
		if prevSymbolTable != nil {
			c.currentSymbolTable = prevSymbolTable
		}
	}()

	// Predefine var header bindings (top-level as global, function scope as local) so body Resolve works
	if vs, ok := node.Variable.(*parser.VarStatement); ok {
		if len(vs.Declarations) > 0 {
			for _, d := range vs.Declarations {
				if c.enclosing == nil {
					idx := c.GetOrAssignGlobalIndex(d.Name.Value)
					c.currentSymbolTable.DefineGlobal(d.Name.Value, idx)
					// fmt.Printf("// [ForInPredefine] var %s as global idx=%d\n", d.Name.Value, idx)
				} else {
					c.currentSymbolTable.Define(d.Name.Value, nilRegister)
					// fmt.Printf("// [ForInPredefine] var %s as local (nil reg)\n", d.Name.Value)
				}
			}
		} else if vs.Name != nil {
			name := vs.Name.Value
			if c.enclosing == nil {
				idx := c.GetOrAssignGlobalIndex(name)
				c.currentSymbolTable.DefineGlobal(name, idx)
				// fmt.Printf("// [ForInPredefine] var %s as global idx=%d\n", name, idx)
			} else {
				c.currentSymbolTable.Define(name, nilRegister)
				// fmt.Printf("// [ForInPredefine] var %s as local (nil reg)\n", name)
			}
		} else {
			// fmt.Printf("// [ForInPredefine] VarStatement without Name/Declarations\n")
		}
	} else if hasLexicalDecl {
		// Define TDZ bindings for let/const variables BEFORE compiling object expression
		// This ensures closures captured in the object expression see the TDZ state
		// We must also emit OpLoadUninitialized to mark the register as TDZ
		switch v := node.Variable.(type) {
		case *parser.LetStatement:
			reg := c.regAlloc.Alloc()
			c.currentSymbolTable.DefineTDZ(v.Name.Value, reg)
			c.emitLoadUninitialized(reg, node.Token.Line)
		case *parser.ConstStatement:
			reg := c.regAlloc.Alloc()
			c.currentSymbolTable.DefineConstTDZ(v.Name.Value, reg)
			c.emitLoadUninitialized(reg, node.Token.Line)
		case *parser.ArrayDestructuringDeclaration:
			for _, elem := range v.Elements {
				if elem.Target == nil {
					continue
				}
				if ident, ok := elem.Target.(*parser.Identifier); ok {
					reg := c.regAlloc.Alloc()
					if v.IsConst {
						c.currentSymbolTable.DefineConstTDZ(ident.Value, reg)
					} else {
						c.currentSymbolTable.DefineTDZ(ident.Value, reg)
					}
					c.emitLoadUninitialized(reg, node.Token.Line)
				}
			}
		case *parser.ObjectDestructuringDeclaration:
			for _, prop := range v.Properties {
				if prop.Target == nil {
					continue
				}
				if ident, ok := prop.Target.(*parser.Identifier); ok {
					reg := c.regAlloc.Alloc()
					if v.IsConst {
						c.currentSymbolTable.DefineConstTDZ(ident.Value, reg)
					} else {
						c.currentSymbolTable.DefineTDZ(ident.Value, reg)
					}
					c.emitLoadUninitialized(reg, node.Token.Line)
				}
			}
		}
	}

	// 1. Compile the object expression first
	objectReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, objectReg)
	_, err := c.compileNode(node.Object, objectReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Get object keys using OpGetOwnKeys
	keysReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, keysReg)
	c.emitOpCode(vm.OpGetOwnKeys, node.Token.Line)
	c.emitByte(byte(keysReg))   // destination register
	c.emitByte(byte(objectReg)) // object register

	// 3. Set up iteration variables (mirroring for...of pattern)
	// We need a key index counter and keys array length
	keyIndexReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, keyIndexReg)
	lengthReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, lengthReg)
	currentKeyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, currentKeyReg)

	// Initialize key index to 0
	c.emitLoadNewConstant(keyIndexReg, vm.Number(0), node.Token.Line)

	// Get length of keys array
	c.emitOpCode(vm.OpGetProp, node.Token.Line)
	c.emitByte(byte(lengthReg)) // destination register
	c.emitByte(byte(keysReg))   // keys array register
	lengthConstIdx := c.chunk.AddConstant(vm.String("length"))
	c.emitUint16(lengthConstIdx) // property name constant index

	// Per ECMAScript spec, initialize completion value V = undefined
	c.emitLoadUndefined(hint, node.Token.Line)

	// --- Loop Start & Context Setup ---
	loopStartPos := len(c.chunk.Code)
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
		CompletionReg:              hint, // For break/continue to update via UpdateEmpty
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// 4. Check if keyIndex < length (loop condition)
	conditionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, conditionReg)
	c.emitOpCode(vm.OpLess, node.Token.Line)
	c.emitByte(byte(conditionReg)) // destination
	c.emitByte(byte(keyIndexReg))  // left operand (key index)
	c.emitByte(byte(lengthReg))    // right operand (length)

	// Jump out of loop if condition is false
	conditionExitJumpPos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)

	// 5. Get current key: keys[keyIndex]
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(currentKeyReg)) // destination
	c.emitByte(byte(keysReg))       // keys array
	c.emitByte(byte(keyIndexReg))   // index

	// 6. Assign current key to loop variable
	// Track per-iteration binding registers for let/const loop variables
	// IMPORTANT: We create a NEW binding here to shadow the TDZ binding, matching for-of behavior
	// This ensures closures captured during object expression evaluation still see the TDZ binding
	var perIterationRegs []Register
	if letStmt, ok := node.Variable.(*parser.LetStatement); ok {
		// Create a NEW binding (shadows the TDZ binding) - matches for-of behavior
		symbol := c.currentSymbolTable.Define(letStmt.Name.Value, c.regAlloc.Alloc())
		c.regAlloc.Pin(symbol.Register) // Pin so closures can capture
		perIterationRegs = append(perIterationRegs, symbol.Register)
		// Store key value in the variable's register
		c.emitMove(symbol.Register, currentKeyReg, node.Token.Line)
	} else if constStmt, ok := node.Variable.(*parser.ConstStatement); ok {
		// Create a NEW const binding (shadows the TDZ binding) - matches for-of behavior
		symbol := c.currentSymbolTable.DefineConst(constStmt.Name.Value, c.regAlloc.Alloc())
		c.regAlloc.Pin(symbol.Register) // Pin so closures can capture
		perIterationRegs = append(perIterationRegs, symbol.Register)
		// Store key value in the variable's register
		c.emitMove(symbol.Register, currentKeyReg, node.Token.Line)
	} else if arrayDestr, ok := node.Variable.(*parser.ArrayDestructuringDeclaration); ok {
		// Array destructuring in for-in loop
		isLexicalBinding := arrayDestr.Token.Type == lexer.LET || arrayDestr.Token.Type == lexer.CONST

		for i, element := range arrayDestr.Elements {
			if element.Target == nil {
				continue
			}

			indexReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, indexReg)
			extractedReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, extractedReg)

			c.emitLoadNewConstant(indexReg, vm.Number(float64(i)), node.Token.Line)
			c.emitOpCode(vm.OpGetIndex, node.Token.Line)
			c.emitByte(byte(extractedReg))
			c.emitByte(byte(currentKeyReg))
			c.emitByte(byte(indexReg))

			if ident, ok := element.Target.(*parser.Identifier); ok {
				var symbol Symbol
				if isLexicalBinding {
					// Create a NEW binding (shadows the TDZ binding) - matches for-of behavior
					// This ensures closures captured during object expression evaluation still see the TDZ binding
					if arrayDestr.IsConst {
						symbol = c.currentSymbolTable.DefineConst(ident.Value, c.regAlloc.Alloc())
					} else {
						symbol = c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
					}
					c.regAlloc.Pin(symbol.Register)
					perIterationRegs = append(perIterationRegs, symbol.Register)
				} else {
					// var binding - define normally
					symbol = c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				}

				// Handle default value if present
				if element.Default != nil {
					err := c.compileConditionalAssignment(element.Target, extractedReg, element.Default, node.Token.Line)
					if err != nil {
						return BadRegister, err
					}
				} else {
					c.emitMove(symbol.Register, extractedReg, node.Token.Line)
				}
			}
		}
	} else if objDestr, ok := node.Variable.(*parser.ObjectDestructuringDeclaration); ok {
		// Object destructuring in for-in loop
		isLexicalBinding := objDestr.Token.Type == lexer.LET || objDestr.Token.Type == lexer.CONST

		for _, prop := range objDestr.Properties {
			if prop.Target == nil {
				continue
			}

			extractedReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, extractedReg)

			// Handle property access (identifier or computed)
			if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
				propConstIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
				c.emitGetProp(extractedReg, currentKeyReg, propConstIdx, node.Token.Line)
			} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
				keyReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, keyReg)
				_, err := c.compileNode(computed.Expr, keyReg)
				if err != nil {
					return BadRegister, err
				}
				c.emitOpCode(vm.OpGetIndex, node.Token.Line)
				c.emitByte(byte(extractedReg))
				c.emitByte(byte(currentKeyReg))
				c.emitByte(byte(keyReg))
			}

			if ident, ok := prop.Target.(*parser.Identifier); ok {
				var symbol Symbol
				if isLexicalBinding {
					// Create a NEW binding (shadows the TDZ binding) - matches for-of behavior
					// This ensures closures captured during object expression evaluation still see the TDZ binding
					if objDestr.IsConst {
						symbol = c.currentSymbolTable.DefineConst(ident.Value, c.regAlloc.Alloc())
					} else {
						symbol = c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
					}
					c.regAlloc.Pin(symbol.Register)
					perIterationRegs = append(perIterationRegs, symbol.Register)
				} else {
					// var binding - define normally
					symbol = c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				}

				// Handle default value if present
				if prop.Default != nil {
					err := c.compileConditionalAssignment(prop.Target, extractedReg, prop.Default, node.Token.Line)
					if err != nil {
						return BadRegister, err
					}
				} else {
					c.emitMove(symbol.Register, extractedReg, node.Token.Line)
				}
			}
		}
	} else if varStmt, ok := node.Variable.(*parser.VarStatement); ok {
		// Handle 'var k in obj' - assign into the pre-defined binding (global at top-level, local in function)
		if varStmt.Name != nil {
			name := varStmt.Name.Value
			// fmt.Printf("// [ForInAssign] assigning to var %s\n", name)
			// Resolve the pre-defined binding from the header
			symbolRef, _, found := c.currentSymbolTable.Resolve(name)
			if !found {
				// As a fallback, predefine now mirroring regular for behavior
				if c.enclosing == nil {
					idx := c.GetOrAssignGlobalIndex(name)
					c.currentSymbolTable.DefineGlobal(name, idx)
					symbolRef, _, found = c.currentSymbolTable.Resolve(name)
				} else {
					c.currentSymbolTable.Define(name, nilRegister)
					symbolRef, _, found = c.currentSymbolTable.Resolve(name)
				}
			}
			if !found {
				return BadRegister, NewCompileError(varStmt, fmt.Sprintf("internal compiler error: failed to resolve var '%s' in for-in", name))
			}
			if symbolRef.IsGlobal {
				// Update global via OpSetGlobal using the assigned global index
				c.emitSetGlobal(symbolRef.GlobalIndex, currentKeyReg, node.Token.Line)
			} else {
				// Local (function-scoped) var: ensure it has a register then move
				if symbolRef.Register == nilRegister {
					reg := c.regAlloc.Alloc()
					// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
					c.currentSymbolTable.UpdateRegister(name, reg)
					symbolRef.Register = reg
				}
				c.emitMove(symbolRef.Register, currentKeyReg, node.Token.Line)
			}
		}
	} else if exprStmt, ok := node.Variable.(*parser.ExpressionStatement); ok {
		// This is an existing variable/pattern being assigned to
		switch target := exprStmt.Expression.(type) {
		case *parser.Identifier:
			// Simple identifier: for (x in obj)
			symbolRef, definingTable, found := c.currentSymbolTable.Resolve(target.Value)
			if !found {
				// fmt.Printf("// [ForInAssign] unresolved %s, defining var in outermost scope\n", target.Value)
				// Define a function/global-scoped binding (var semantics)
				if c.enclosing == nil {
					idx := c.GetOrAssignGlobalIndex(target.Value)
					c.currentSymbolTable.DefineGlobal(target.Value, idx)
					c.emitSetGlobal(idx, currentKeyReg, node.Token.Line)
				} else {
					scope := c.currentSymbolTable
					for scope.Outer != nil {
						scope = scope.Outer
					}
					reg := c.regAlloc.Alloc()
					tempRegs = append(tempRegs, reg)
					sym := scope.Define(target.Value, reg)
					// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
					c.emitMove(sym.Register, currentKeyReg, node.Token.Line)
				}
			} else {
				// Check if this is a global variable or local register
				if symbolRef.IsGlobal {
					// fmt.Printf("// [ForInAssign] writing global %s\n", target.Value)
					c.emitSetGlobal(symbolRef.GlobalIndex, currentKeyReg, node.Token.Line)
				} else {
					// Store key value in the existing variable's register for local variables
					_ = definingTable
					// fmt.Printf("// [ForInAssign] writing local %s R%d\n", target.Value, symbolRef.Register)
					c.emitMove(symbolRef.Register, currentKeyReg, node.Token.Line)
				}
			}
		case *parser.ArrayLiteral:
			// Array destructuring assignment: for ([x, y] in obj)
			if err := c.compileNestedArrayDestructuring(target, currentKeyReg, node.Token.Line); err != nil {
				return BadRegister, err
			}
		case *parser.ObjectLiteral:
			// Object destructuring assignment: for ({a, b} in obj)
			if err := c.compileNestedObjectDestructuring(target, currentKeyReg, node.Token.Line); err != nil {
				return BadRegister, err
			}
		case *parser.MemberExpression:
			// Member expression: for (obj.x in items) or for (obj[key] in items)
			if err := c.compileAssignmentToMember(target, currentKeyReg, node.Token.Line); err != nil {
				return BadRegister, err
			}
		case *parser.IndexExpression:
			// Index expression: for (arr[idx] in items) or for ([let][1] in items)
			if err := c.compileAssignmentToIndex(target, currentKeyReg, node.Token.Line); err != nil {
				return BadRegister, err
			}
		}
	}

	// 7. Compile loop body
	bodyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, bodyReg)
	resultReg, err := c.compileNode(node.Body, bodyReg)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, err
	}
	// If the body produced a value, update completion value V
	if resultReg != BadRegister {
		c.emitMove(hint, resultReg, node.Token.Line)
	}

	// 8. Patch continue jumps to land here (before increment)
	for _, continuePos := range loopContext.ContinuePlaceholderPosList {
		c.patchJump(continuePos)
	}

	// 8a. Per-iteration bindings: close upvalues for let/const loop variables
	// ECMAScript spec: each iteration gets fresh bindings. This ensures closures
	// capture the value at this iteration, not the final value.
	for _, reg := range perIterationRegs {
		c.emitCloseUpvalue(reg, node.Token.Line)
	}

	// 9. Increment key index
	oneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, oneReg)
	c.emitLoadNewConstant(oneReg, vm.Number(1), node.Token.Line)
	c.emitOpCode(vm.OpAdd, node.Token.Line)
	c.emitByte(byte(keyIndexReg)) // destination (reuse keyIndexReg)
	c.emitByte(byte(keyIndexReg)) // left operand (current key index)
	c.emitByte(byte(oneReg))      // right operand (1)

	// 10. Jump back to loop start
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2
	backOffset := loopStartPos - jumpBackInstructionEndPos
	c.emitOpCode(vm.OpJump, node.Body.Token.Line)
	c.emitUint16(uint16(int16(backOffset)))

	// 11. Clean up loop context and patch jumps
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	// Patch condition exit jump
	c.patchJump(conditionExitJumpPos)
	patchedConditionExit = true

	// Patch all break jumps
	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos)
	}

	// Return completion value
	return hint, nil
}

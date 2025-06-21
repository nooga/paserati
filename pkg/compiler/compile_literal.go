package compiler

import (
	"fmt"
	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

// compileArrowFunctionLiteral compiles an arrow function literal expression.
func (c *Compiler) compileArrowFunctionLiteral(node *parser.ArrowFunctionLiteral, hint Register) (Register, errors.PaseratiError) {
	// 1. Create a compiler for the function scope
	funcCompiler := newFunctionCompiler(c)
	funcCompiler.compilingFuncName = "<arrow>" // Set name for arrow functions

	// 2. Define parameters in the function's symbol table
	for _, p := range node.Parameters {
		reg := funcCompiler.regAlloc.Alloc()
		// --- FIX: Access Name field ---
		funcCompiler.currentSymbolTable.Define(p.Name.Value, reg)
		// Pin the register since parameters can be captured by inner functions
		funcCompiler.regAlloc.Pin(reg)
	}

	// 3. Handle default parameters
	for _, param := range node.Parameters {
		if param.DefaultValue != nil {
			// Get the parameter's register
			symbol, _, exists := funcCompiler.currentSymbolTable.Resolve(param.Name.Value)
			if !exists {
				// This should not happen if parameter definition worked correctly
				funcCompiler.addError(param.Name, fmt.Sprintf("parameter %s not found in symbol table", param.Name.Value))
				continue
			}
			paramReg := symbol.Register

			// Create temporary registers for comparison
			undefinedReg := funcCompiler.regAlloc.Alloc()
			defer funcCompiler.regAlloc.Free(undefinedReg)
			funcCompiler.emitLoadUndefined(undefinedReg, param.Token.Line)

			compareReg := funcCompiler.regAlloc.Alloc()
			defer funcCompiler.regAlloc.Free(compareReg)
			funcCompiler.emitStrictEqual(compareReg, paramReg, undefinedReg, param.Token.Line)

			// Jump if the comparison is false (parameter is not undefined, so keep original value)
			jumpIfDefinedPos := funcCompiler.emitPlaceholderJump(vm.OpJumpIfFalse, compareReg, param.Token.Line)

			// Compile the default value expression
			defaultValueReg := funcCompiler.regAlloc.Alloc()
			_, err := funcCompiler.compileNode(param.DefaultValue, defaultValueReg)
			if err != nil {
				// Continue with compilation even if default value has errors
				funcCompiler.addError(param.DefaultValue, fmt.Sprintf("error compiling default value for parameter %s", param.Name.Value))
			} else {
				// Move the default value to the parameter register
				if defaultValueReg != paramReg {
					funcCompiler.emitMove(paramReg, defaultValueReg, param.Token.Line)
				}
			}
			funcCompiler.regAlloc.Free(defaultValueReg)

			// Patch the jump to come here (end of default value assignment)
			funcCompiler.patchJump(jumpIfDefinedPos)
		}
	}

	// 4. Handle rest parameter (if present)
	if node.RestParameter != nil {
		// Define the rest parameter in the symbol table
		restParamReg := funcCompiler.regAlloc.Alloc()
		funcCompiler.currentSymbolTable.Define(node.RestParameter.Name.Value, restParamReg)
		// Pin the register since rest parameters can be captured by inner functions
		funcCompiler.regAlloc.Pin(restParamReg)

		// The rest parameter collection will be handled at runtime during function call
		// We just need to ensure it has a register allocated here
		debugPrintf("// [Compiler] Rest parameter '%s' defined in R%d\n", node.RestParameter.Name.Value, restParamReg)
	}

	// 5. Compile the function body
	var returnReg Register
	implicitReturnNeeded := true
	switch bodyNode := node.Body.(type) {
	case *parser.BlockStatement:
		bodyResultReg := funcCompiler.regAlloc.Alloc()
		_, err := funcCompiler.compileNode(bodyNode, bodyResultReg)
		funcCompiler.regAlloc.Free(bodyResultReg)
		if err != nil {
			funcCompiler.errors = append(funcCompiler.errors, err)
		}
		implicitReturnNeeded = false // Block handles its own returns or falls through
	case parser.Expression:
		returnReg = funcCompiler.regAlloc.Alloc()
		_, err := funcCompiler.compileNode(bodyNode, returnReg)
		if err != nil {
			funcCompiler.errors = append(funcCompiler.errors, err)
			returnReg = 0 // Indicate error or inability to get result reg
		}
		implicitReturnNeeded = true // Expression body needs implicit return
	default:
		funcCompiler.errors = append(funcCompiler.errors, NewCompileError(node, fmt.Sprintf("invalid body type %T for arrow function", node.Body)))
		implicitReturnNeeded = false
	}
	if implicitReturnNeeded {
		funcCompiler.emitReturn(returnReg, node.Token.Line)
		funcCompiler.regAlloc.Free(returnReg)
	}

	// Add final implicit return for the function (catches paths that don't hit explicit/implicit returns)
	funcCompiler.emitFinalReturn(node.Token.Line) // Use arrow token line number

	// Collect errors from sub-compilation
	if len(funcCompiler.errors) > 0 {
		c.errors = append(c.errors, funcCompiler.errors...)
		// Continue even with errors to potentially catch more issues
	}

	// Get captured free variables and required register count
	freeSymbols := funcCompiler.freeSymbols
	regSize := funcCompiler.regAlloc.MaxRegs()
	functionChunk := funcCompiler.chunk

	// <<< ADDED: Debug dump function bytecode >>>
	if debugCompiler {
		fmt.Printf("\n=== Function Bytecode: %s ===\n", funcCompiler.compilingFuncName)
		fmt.Print(functionChunk.DisassembleChunk(funcCompiler.compilingFuncName))
		fmt.Printf("=== END %s ===\n\n", funcCompiler.compilingFuncName)
	}
	// <<< END ADDED >>>

	// 6. Create the function object directly using vm.NewFunction
	// Count parameters excluding 'this' parameters for arity calculation
	arity := 0
	for _, param := range node.Parameters {
		if !param.IsThis {
			arity++
		}
	}
	funcValue := vm.NewFunction(arity, len(freeSymbols), int(regSize), node.RestParameter != nil, "<arrow>", functionChunk)
	constIdx := c.chunk.AddConstant(funcValue)

	// 8. Emit OpClosure in the *enclosing* compiler (c) - result goes to hint register
	debugPrintf("// [Closure %s] Using hint register: R%d\n", funcCompiler.compilingFuncName, hint)
	c.emitOpCode(vm.OpClosure, node.Token.Line)
	c.emitByte(byte(hint))
	c.emitUint16(constIdx)             // Operand 1: Constant index of the function blueprint
	c.emitByte(byte(len(freeSymbols))) // Operand 2: Number of upvalues to capture

	// Emit operands for each upvalue
	for i, freeSym := range freeSymbols {
		debugPrintf("// [Closure Loop %s] Checking freeSym[%d]: %s (Reg %d) against funcNameForLookup: '%s'\n", funcCompiler.compilingFuncName, i, freeSym.Name, freeSym.Register, funcCompiler.compilingFuncName)

		// --- Check for self-capture first (Simplified Check) ---
		// If a free symbol has nilRegister, it MUST be the temporary one
		// added for recursion resolution. It signifies self-capture.
		if freeSym.Register == nilRegister {
			// This is the special self-capture case identified during body compilation.
			debugPrintf("// [Closure SelfCapture %s] Symbol '%s' has nilRegister. Emitting isLocal=1, index=hint=R%d\n", funcCompiler.compilingFuncName, freeSym.Name, hint)
			c.emitByte(1)          // isLocal = true (capture from the stack where the closure will be placed)
			c.emitByte(byte(hint)) // Index = the hint register of OpClosure
			continue               // Skip the normal lookup below
		}
		// --- END Check ---

		// Resolve the symbol again in the *enclosing* compiler's context
		// (This part should now only run for *non-recursive* free variables)
		enclosingSymbol, enclosingTable, found := c.currentSymbolTable.Resolve(freeSym.Name)
		if !found {
			// This should theoretically not happen if it was resolved during body compilation
			// but handle defensively.
			panic(fmt.Sprintf("compiler internal error: free variable %s not found in enclosing scope during closure creation", freeSym.Name))
		}

		if enclosingTable == c.currentSymbolTable {
			debugPrintf("// [Closure Loop %s] Free '%s' is Local in enclosing. Emitting isLocal=1, index=R%d\n", funcCompiler.compilingFuncName, freeSym.Name, enclosingSymbol.Register)
			// The free variable is local in the *direct* enclosing scope.
			c.emitByte(1) // isLocal = true
			// Capture the value from the enclosing scope's actual register
			c.emitByte(byte(enclosingSymbol.Register)) // Index = register index
		} else {
			// The free variable is also a free variable in the enclosing scope.
			// We need to capture it from the enclosing scope's upvalues.
			// We need the index of this symbol within the *enclosing* compiler's freeSymbols list.
			enclosingFreeIndex := c.addFreeSymbol(node, &enclosingSymbol)
			debugPrintf("// [Closure Loop %s] Free '%s' is Outer in enclosing. Emitting isLocal=0, index=%d\n", funcCompiler.compilingFuncName, freeSym.Name, enclosingFreeIndex)
			c.emitByte(0)                        // isLocal = false
			c.emitByte(byte(enclosingFreeIndex)) // Index = upvalue index in enclosing scope
		}
	}

	return hint, nil // Return hint register with the closure
}

func (c *Compiler) compileArrayLiteral(node *parser.ArrayLiteral, hint Register) (Register, errors.PaseratiError) {
	elementCount := len(node.Elements)
	if elementCount > 255 { // Check against OpMakeArray count operand size
		return BadRegister, NewCompileError(node, "array literal exceeds maximum size of 255 elements")
	}

	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile elements and store their final registers
	elementRegs := make([]Register, elementCount)
	elementRegsContinuous := true
	for i, elem := range node.Elements {
		elemReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, elemReg)
		_, err := c.compileNode(elem, elemReg)
		if err != nil {
			return BadRegister, err
		}
		elementRegs[i] = elemReg // Store the register holding the final result of this element
		if i > 0 && elementRegs[i] != elementRegs[i-1]+1 {
			elementRegsContinuous = false
		}
	}

	// 2. Allocate a contiguous block for the elements and move them
	var firstTargetReg Register
	if elementCount > 0 && elementRegsContinuous {
		firstTargetReg = elementRegs[0]
	} else if elementCount > 0 {
		// Allocate a contiguous block for all elements
		firstTargetReg = c.regAlloc.AllocContiguous(elementCount)
		// Mark all registers in the block for cleanup
		for i := 0; i < elementCount; i++ {
			tempRegs = append(tempRegs, firstTargetReg+Register(i))
		}

		// Move elements from their original registers (elementRegs)
		// into the new contiguous block starting at firstTargetReg.
		for i := 0; i < elementCount; i++ {
			targetReg := firstTargetReg + Register(i)
			sourceReg := elementRegs[i]
			if sourceReg != targetReg { // Avoid redundant moves
				c.emitMove(targetReg, sourceReg, node.Token.Line)
			}
		}
	} else {
		// Handle empty array case: OpMakeArray needs a StartReg, use 0.
		firstTargetReg = 0
	}

	// 3. Emit OpMakeArray using the contiguous block - result goes to hint
	c.emitOpCode(vm.OpMakeArray, node.Token.Line)
	c.emitByte(byte(hint))           // DestReg: where the new array object goes (hint)
	c.emitByte(byte(firstTargetReg)) // StartReg: start of the contiguous element block
	c.emitByte(byte(elementCount))   // Count: number of elements

	// Result (the array) is now in hint register
	return hint, nil
}

func (c *Compiler) compileObjectLiteral(node *parser.ObjectLiteral, hint Register) (Register, errors.PaseratiError) {
	debugPrintf("Compiling Object Literal (One-by-One): %s\n", node.String())
	line := parser.GetTokenFromNode(node).Line

	// 1. Create an empty object in hint register
	c.emitMakeEmptyObject(hint, line)

	// 2. Set properties one by one
	for _, prop := range node.Properties {
		// Compile Key (must evaluate to string constant for OpSetProp in Phase 1)
		var keyConstIdx uint16 = 0xFFFF // Invalid index marker
		switch keyNode := prop.Key.(type) {
		case *parser.Identifier:
			keyStr := keyNode.Value
			keyConstIdx = c.chunk.AddConstant(vm.String(keyStr))
		case *parser.StringLiteral:
			keyStr := keyNode.Value
			keyConstIdx = c.chunk.AddConstant(vm.String(keyStr))
		case *parser.NumberLiteral: // Allow number literal keys, convert to string
			keyStr := keyNode.TokenLiteral()
			keyConstIdx = c.chunk.AddConstant(vm.String(keyStr))
		default:
			// TODO: Handle computed keys [expr]. For Phase 1, only Ident/String/Number keys.
			// Computed keys would require compiling the expression, ensuring it's a string/number,
			// and potentially a different OpSetComputedProp or dynamic lookup within OpSetProp.
			return BadRegister, NewCompileError(prop.Key, fmt.Sprintf("compiler only supports identifier, string, or number literal keys in object literals (Phase 1), got %T", prop.Key))
		}

		// Compile Value into a temporary register
		valueReg := c.regAlloc.Alloc()
		_, err := c.compileNode(prop.Value, valueReg)
		if err != nil {
			c.regAlloc.Free(valueReg)
			return BadRegister, err
		}
		debugPrintf("--- OL Value Compiled. valueReg: R%d\n", valueReg)

		// Emit OpSetProp: hint[keyConstIdx] = valueReg
		c.emitSetProp(hint, valueReg, keyConstIdx, line)

		// Free the temporary value register
		c.regAlloc.Free(valueReg)
	}

	// The object is fully constructed in hint register
	return hint, nil
}

func (c *Compiler) compileTemplateLiteral(node *parser.TemplateLiteral, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line
	parts := node.Parts

	// Handle edge case: empty template ``
	if len(parts) == 0 {
		// Empty template becomes empty string
		c.emitLoadNewConstant(hint, vm.String(""), line)
		return hint, nil
	}

	// Handle single part (just a string)
	if len(parts) == 1 {
		if stringPart, ok := parts[0].(*parser.TemplateStringPart); ok {
			c.emitLoadNewConstant(hint, vm.String(stringPart.Value), line)
			return hint, nil
		}
		// Single interpolated expression: convert to string
		_, err := c.compileNode(parts[0], hint)
		if err != nil {
			return BadRegister, err
		}
		// Result is already in hint register, but we might need to convert to string
		// For now, assume expressions evaluate to their string representation
		// TODO: Add explicit string conversion if needed
		return hint, nil
	}

	// Multiple parts: build up result using binary concatenation
	var resultReg Register
	var initialized bool = false

	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	for _, part := range parts {
		switch p := part.(type) {
		case *parser.TemplateStringPart:
			// String part: load as constant
			stringReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, stringReg)
			c.emitLoadNewConstant(stringReg, vm.String(p.Value), line)

			if !initialized {
				resultReg = stringReg
				initialized = true
			} else {
				// Concatenate with previous result
				newResultReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, newResultReg)
				c.emitStringConcat(newResultReg, resultReg, stringReg, line)
				resultReg = newResultReg
			}

		default:
			// Expression part: compile and concatenate
			exprReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, exprReg)
			_, err := c.compileNode(p, exprReg)
			if err != nil {
				return BadRegister, err
			}

			if !initialized {
				resultReg = exprReg
				initialized = true
			} else {
				// Concatenate with previous result
				newResultReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, newResultReg)
				c.emitStringConcat(newResultReg, resultReg, exprReg, line)
				resultReg = newResultReg
			}
		}
	}

	// Move final result to hint register if it's different
	if resultReg != hint {
		c.emitMove(hint, resultReg, line)
	}

	return hint, nil
}

// --- Modify signature again to return (uint16, []*Symbol, errors.PaseratiError) ---
func (c *Compiler) compileFunctionLiteral(node *parser.FunctionLiteral, nameHint string) (uint16, []*Symbol, errors.PaseratiError) {
	// 1. Create a new Compiler instance for the function body, linked to the current one
	functionCompiler := newFunctionCompiler(c) // <<< Keep this instance variable

	// ... (rest of the function setup: determine name, define inner name, define params) ...
	// --- Determine and set the function name being compiled ---
	var determinedFuncName string
	if nameHint != "" {
		determinedFuncName = nameHint
	} else if node.Name != nil {
		determinedFuncName = node.Name.Value
	} else {
		determinedFuncName = "<anonymous>"
	}
	functionCompiler.compilingFuncName = determinedFuncName
	// --- End Set Name ---

	// --- NEW: Define inner name in inner scope for recursion ---
	var funcNameForLookup string // Name used for potential recursive lookup
	if node.Name != nil {
		funcNameForLookup = node.Name.Value
		// Define the function's own name within its scope temporarily
		functionCompiler.currentSymbolTable.Define(funcNameForLookup, nilRegister)
	} else if nameHint != "" {
		// If anonymous but assigned (e.g., let f = function() { f(); }),
		funcNameForLookup = nameHint
		functionCompiler.currentSymbolTable.Define(funcNameForLookup, nilRegister)
	}
	// --- END NEW ---

	debugPrintf("// [Compiling Function Literal] %s\n", determinedFuncName)

	// 2. Define parameters in the function compiler's *enclosed* scope
	for _, param := range node.Parameters {
		// Skip 'this' parameters - they don't have names and don't get compiled as regular parameters
		if param.IsThis {
			continue
		}
		reg := functionCompiler.regAlloc.Alloc()
		functionCompiler.currentSymbolTable.Define(param.Name.Value, reg)
		// Pin the register since parameters can be captured by inner functions
		functionCompiler.regAlloc.Pin(reg)
		debugPrintf("// [Compiling Function Literal] %s: Parameter %s defined in R%d\n", determinedFuncName, param.Name.Value, reg)
	}

	// 3. Handle default parameters
	for _, param := range node.Parameters {
		// Skip 'this' parameters - they don't have default values and aren't in the symbol table
		if param.IsThis {
			continue
		}
		if param.DefaultValue != nil {
			// Get the parameter's register
			symbol, _, exists := functionCompiler.currentSymbolTable.Resolve(param.Name.Value)
			if !exists {
				// This should not happen if parameter definition worked correctly
				functionCompiler.addError(param.Name, fmt.Sprintf("parameter %s not found in symbol table", param.Name.Value))
				continue
			}
			paramReg := symbol.Register

			// Create a temporary register to hold undefined for comparison
			undefinedReg := functionCompiler.regAlloc.Alloc()
			functionCompiler.emitLoadUndefined(undefinedReg, param.Token.Line)

			// Create another temporary register for the comparison result
			compareReg := functionCompiler.regAlloc.Alloc()
			functionCompiler.emitStrictEqual(compareReg, paramReg, undefinedReg, param.Token.Line)

			// Jump if the comparison is false (parameter is not undefined, so keep original value)
			jumpIfDefinedPos := functionCompiler.emitPlaceholderJump(vm.OpJumpIfFalse, compareReg, param.Token.Line)

			// Free temporary registers
			functionCompiler.regAlloc.Free(undefinedReg)
			functionCompiler.regAlloc.Free(compareReg)

			// Compile the default value expression
			defaultValueReg := functionCompiler.regAlloc.Alloc()
			_, err := functionCompiler.compileNode(param.DefaultValue, defaultValueReg)
			if err != nil {
				// Continue with compilation even if default value has errors
				functionCompiler.addError(param.DefaultValue, fmt.Sprintf("error compiling default value for parameter %s", param.Name.Value))
			} else {
				// Move the default value to the parameter register
				if defaultValueReg != paramReg {
					functionCompiler.emitMove(paramReg, defaultValueReg, param.Token.Line)
					// Free the temporary default value register after moving
					functionCompiler.regAlloc.Free(defaultValueReg)
				}
			}

			// Patch the jump to come here (end of default value assignment)
			functionCompiler.patchJump(jumpIfDefinedPos)
		}
	}

	// 4. Handle rest parameter (if present)
	if node.RestParameter != nil {
		// Define the rest parameter in the symbol table
		restParamReg := functionCompiler.regAlloc.Alloc()
		functionCompiler.currentSymbolTable.Define(node.RestParameter.Name.Value, restParamReg)
		// Pin the register since rest parameters can be captured by inner functions
		functionCompiler.regAlloc.Pin(restParamReg)

		// The rest parameter collection will be handled at runtime during function call
		// We just need to ensure it has a register allocated here
		debugPrintf("// [Compiler] Rest parameter '%s' defined in R%d\n", node.RestParameter.Name.Value, restParamReg)
	}

	// 5. Compile the body using the function compiler
	bodyReg := functionCompiler.regAlloc.Alloc()
	_, err := functionCompiler.compileNode(node.Body, bodyReg)
	functionCompiler.regAlloc.Free(bodyReg) // Free since function body doesn't return a value
	if err != nil {
		// Propagate errors (already appended to c.errors by sub-compiler)
		// Proceed to create function object even if body has errors? Continue for now.
	}

	// 6. Finalize function chunk (add implicit return to the function's chunk)
	functionCompiler.emitFinalReturn(node.Body.Token.Line) // Use body's end token? Or func literal token?
	functionChunk := functionCompiler.chunk

	// <<< ADDED: Debug dump function bytecode >>>
	if debugCompiler {
		fmt.Printf("\n=== Function Bytecode: %s ===\n", determinedFuncName)
		fmt.Print(functionChunk.DisassembleChunk(determinedFuncName))
		fmt.Printf("=== END %s ===\n\n", determinedFuncName)
	}
	// <<< END ADDED >>>

	// <<< Get freeSymbols from the functionCompiler instance >>>
	freeSymbols := functionCompiler.freeSymbols
	// Collect any additional errors from the sub-compilation
	if len(functionCompiler.errors) > 0 {
		c.errors = append(c.errors, functionCompiler.errors...)
	}
	regSize := functionCompiler.regAlloc.MaxRegs()

	// 7. Create the bytecode.Function object
	// ... (determine funcName as before) ...
	var funcName string
	if nameHint != "" {
		funcName = nameHint
	} else if node.Name != nil {
		funcName = node.Name.Value
	} else {
		funcName = "<anonymous>"
	}

	// 8. Add the function object to the *outer* compiler's constant pool.
	// Count parameters excluding 'this' parameters for arity calculation
	arity := 0
	for _, param := range node.Parameters {
		if !param.IsThis {
			arity++
		}
	}
	funcValue := vm.NewFunction(arity, len(freeSymbols), int(regSize), node.RestParameter != nil, funcName, functionChunk)
	constIdx := c.chunk.AddConstant(funcValue)

	// <<< REMOVE OpClosure EMISSION FROM HERE (should already be removed) >>>

	// --- Return the constant index, the free symbols, and nil error ---
	// Accumulated errors are in c.errors.
	return constIdx, freeSymbols, nil // <<< MODIFY return statement
}

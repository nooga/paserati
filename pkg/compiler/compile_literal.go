package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// isFutureReservedWord checks if an identifier is a FutureReservedWord
// These are reserved in strict mode but allowed as identifiers in non-strict mode
// Note: 'yield' and 'let' are handled specially by the parser and are not included here
var futureReservedWords = map[string]bool{
	"implements": true,
	"interface":  true,
	"package":    true,
	"private":    true,
	"protected":  true,
	"public":     true,
	"static":     true,
}

func isFutureReservedWord(name string) bool {
	return futureReservedWords[name]
}

// compileArrowFunctionLiteral compiles an arrow function literal expression.
func (c *Compiler) compileArrowFunctionLiteral(node *parser.ArrowFunctionLiteral, hint Register) (Register, errors.PaseratiError) {
	// 1. Create a compiler for the function scope
	funcCompiler := newFunctionCompiler(c)
	funcCompiler.compilingFuncName = "<arrow>" // Set name for arrow functions
	funcCompiler.isArrowFunction = true        // Arrow functions don't have own 'arguments' binding

	// 1.5. Check for 'use strict' directive in arrow function body (if it's a block statement)
	if blockBody, ok := node.Body.(*parser.BlockStatement); ok {
		if hasStrictDirective(blockBody) {
			funcCompiler.chunk.IsStrict = true
			debugPrintf("// [compileArrowFunctionLiteral] Detected 'use strict' directive in arrow function body\n")
		}
	}

	// 2. Define parameters in the function's symbol table
	// Track seen parameter names for duplicate detection in strict mode
	seenArrowParams := make(map[string]bool)

	for _, p := range node.Parameters {
		// Strict mode validation: cannot use 'eval' or 'arguments' as parameter names
		if funcCompiler.chunk.IsStrict && (p.Name.Value == "eval" || p.Name.Value == "arguments") {
			funcCompiler.addError(p.Name, fmt.Sprintf("SyntaxError: Strict mode function may not have parameter named '%s'", p.Name.Value))
			// Continue defining it to avoid cascading errors
		}

		// Strict mode validation: FutureReservedWords cannot be used as parameter names
		if funcCompiler.chunk.IsStrict && isFutureReservedWord(p.Name.Value) {
			funcCompiler.addError(p.Name, fmt.Sprintf("SyntaxError: Unexpected strict mode reserved word '%s'", p.Name.Value))
		}

		// Strict mode validation: duplicate parameter names are forbidden
		if funcCompiler.chunk.IsStrict {
			if seenArrowParams[p.Name.Value] {
				funcCompiler.addError(p.Name, fmt.Sprintf("SyntaxError: Duplicate parameter name '%s' not allowed in strict mode", p.Name.Value))
			}
			seenArrowParams[p.Name.Value] = true
		}

		reg := funcCompiler.regAlloc.Alloc()
		// --- FIX: Access Name field ---
		funcCompiler.currentSymbolTable.Define(p.Name.Value, reg)
		// Pin the register since parameters can be captured by inner functions
		funcCompiler.regAlloc.Pin(reg)
	}

	// 3. Handle default parameters
	// Build parameter list for TDZ checking
	funcCompiler.parameterList = make([]string, len(node.Parameters))
	for i, p := range node.Parameters {
		funcCompiler.parameterList[i] = p.Name.Value
	}

	for paramIdx, param := range node.Parameters {
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
			// Set TDZ tracking: parameters at index >= paramIdx are in TDZ
			defaultValueReg := funcCompiler.regAlloc.Alloc()
			funcCompiler.currentDefaultParamIndex = paramIdx
			funcCompiler.inDefaultParamScope = true
			_, err := funcCompiler.compileNode(param.DefaultValue, defaultValueReg)
			funcCompiler.inDefaultParamScope = false
			funcCompiler.currentDefaultParamIndex = -1
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
		// Handle both simple rest parameters (...args) and destructured (...[x, y])
		if node.RestParameter.Name != nil {
			funcCompiler.currentSymbolTable.Define(node.RestParameter.Name.Value, restParamReg)
			debugPrintf("// [Compiler] Rest parameter '%s' defined in R%d\n", node.RestParameter.Name.Value, restParamReg)
		} else if node.RestParameter.Pattern != nil {
			// For destructured rest parameters, we'll define a temporary and handle destructuring
			funcCompiler.currentSymbolTable.Define("__rest__", restParamReg)
			debugPrintf("// [Compiler] Rest parameter (destructured) defined in R%d\n", restParamReg)
		}
		// Pin the register since rest parameters can be captured by inner functions
		funcCompiler.regAlloc.Pin(restParamReg)
	}

	// 5. Compile the function body
	var returnReg Register
	implicitReturnNeeded := true
	switch bodyNode := node.Body.(type) {
	case *parser.BlockStatement:
		bodyResultReg := funcCompiler.regAlloc.Alloc()
		// Mark that we're compiling the function body BlockStatement itself
		funcCompiler.isCompilingFunctionBody = true
		_, err := funcCompiler.compileNode(bodyNode, bodyResultReg)
		funcCompiler.isCompilingFunctionBody = false
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
	// Arity: total parameters excluding 'this' (for VM register allocation)
	// Length: params before first default (for ECMAScript function.length property)
	arity := 0
	length := 0
	seenDefault := false
	for _, param := range node.Parameters {
		if param.IsThis {
			continue
		}
		arity++
		if !seenDefault && param.DefaultValue == nil {
			length++
		} else {
			seenDefault = true
		}
	}
	arrowName := "<arrow>"
	functionChunk.NumSpillSlots = int(funcCompiler.nextSpillSlot)                                                                                              // Set spill slots needed
	funcValue := vm.NewFunction(arity, length, len(freeSymbols), int(regSize), node.RestParameter != nil, arrowName, functionChunk, false, node.IsAsync, true) // isArrowFunction = true
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

		// Check if the variable is from an ENCLOSING compiler's scope (a grandparent relative to the arrow).
		// The Outer chain of symbol tables crosses compiler boundaries, so we can't just walk
		// c.currentSymbolTable.Outer to determine if it's "within" the current function.
		// Instead, use isDefinedInEnclosingCompiler to check if enclosingTable belongs to
		// any of c's enclosing compilers (grandparent and beyond).
		isInOuterCompiler := c.enclosing != nil && c.isDefinedInEnclosingCompiler(enclosingTable)

		if enclosingTable == c.currentSymbolTable || !isInOuterCompiler {
			// The free variable is within the enclosing function's scope (possibly in an outer block).
			if enclosingSymbol.IsSpilled {
				// Spilled variable: capture from spill slot
				debugPrintf("// [Closure Loop %s] Free '%s' is SPILLED (slot %d), emitting capture from spill\n", funcCompiler.compilingFuncName, freeSym.Name, enclosingSymbol.SpillIndex)
				if enclosingSymbol.SpillIndex <= 255 {
					c.emitByte(2) // isLocal = 2 means spill slot (8-bit index)
					c.emitByte(byte(enclosingSymbol.SpillIndex))
				} else {
					c.emitByte(3) // isLocal = 3 means spill slot (16-bit index)
					c.emitUint16(enclosingSymbol.SpillIndex)
				}
			} else {
				debugPrintf("// [Closure Loop %s] Free '%s' is in current function's scope chain. Emitting isLocal=1, index=R%d\n", funcCompiler.compilingFuncName, freeSym.Name, enclosingSymbol.Register)
				c.emitByte(1) // isLocal = true
				// Capture the value from the enclosing scope's actual register
				c.emitByte(byte(enclosingSymbol.Register)) // Index = register index
			}
		} else {
			// The free variable is in an outer function's scope (grandparent or beyond).
			// It must be a free variable in the enclosing function as well.
			// We need to capture it from the enclosing scope's upvalues.
			enclosingFreeIndex := c.addFreeSymbol(node, &enclosingSymbol)
			debugPrintf("// [Closure Loop %s] Free '%s' is in outer function scope. Emitting isLocal=0, index=%d\n", funcCompiler.compilingFuncName, freeSym.Name, enclosingFreeIndex)
			c.emitByte(0)                        // isLocal = false
			c.emitByte(byte(enclosingFreeIndex)) // Index = upvalue index in enclosing scope
		}
	}

	return hint, nil // Return hint register with the closure
}

// compileArrowFunctionWithName compiles an arrow function with a custom name hint
// This is used for function name inference in destructuring defaults
func (c *Compiler) compileArrowFunctionWithName(node *parser.ArrowFunctionLiteral, nameHint string) (uint16, []*Symbol, errors.PaseratiError) {
	// Most of this is copied from compileArrowFunctionLiteral, but with name support
	funcCompiler := newFunctionCompiler(c)
	funcCompiler.compilingFuncName = nameHint // Use the provided name instead of "<arrow>"
	funcCompiler.isArrowFunction = true       // Arrow functions don't have own 'arguments' binding

	// Define parameters
	for _, p := range node.Parameters {
		reg := funcCompiler.regAlloc.Alloc()
		funcCompiler.currentSymbolTable.Define(p.Name.Value, reg)
		funcCompiler.regAlloc.Pin(reg)
	}

	// Handle default parameters (same as original)
	// Build parameter list for TDZ checking
	funcCompiler.parameterList = make([]string, len(node.Parameters))
	for i, p := range node.Parameters {
		funcCompiler.parameterList[i] = p.Name.Value
	}

	for paramIdx, param := range node.Parameters {
		if param.DefaultValue != nil {
			symbol, _, exists := funcCompiler.currentSymbolTable.Resolve(param.Name.Value)
			if !exists {
				funcCompiler.addError(param.Name, fmt.Sprintf("parameter %s not found in symbol table", param.Name.Value))
				continue
			}
			paramReg := symbol.Register

			undefinedReg := funcCompiler.regAlloc.Alloc()
			defer funcCompiler.regAlloc.Free(undefinedReg)
			funcCompiler.emitLoadUndefined(undefinedReg, param.Token.Line)

			compareReg := funcCompiler.regAlloc.Alloc()
			defer funcCompiler.regAlloc.Free(compareReg)
			funcCompiler.emitStrictEqual(compareReg, paramReg, undefinedReg, param.Token.Line)

			jumpIfDefinedPos := funcCompiler.emitPlaceholderJump(vm.OpJumpIfFalse, compareReg, param.Token.Line)

			// Set TDZ tracking: parameters at index >= paramIdx are in TDZ
			defaultValueReg := funcCompiler.regAlloc.Alloc()
			funcCompiler.currentDefaultParamIndex = paramIdx
			funcCompiler.inDefaultParamScope = true
			_, err := funcCompiler.compileNode(param.DefaultValue, defaultValueReg)
			funcCompiler.inDefaultParamScope = false
			funcCompiler.currentDefaultParamIndex = -1
			if err != nil {
				funcCompiler.addError(param.DefaultValue, fmt.Sprintf("error compiling default value for parameter %s", param.Name.Value))
			} else {
				if defaultValueReg != paramReg {
					funcCompiler.emitMove(paramReg, defaultValueReg, param.Token.Line)
				}
			}
			funcCompiler.regAlloc.Free(defaultValueReg)
			funcCompiler.patchJump(jumpIfDefinedPos)
		}
	}

	// Handle rest parameter (same as original)
	if node.RestParameter != nil {
		restParamReg := funcCompiler.regAlloc.Alloc()
		if node.RestParameter.Name != nil {
			funcCompiler.currentSymbolTable.Define(node.RestParameter.Name.Value, restParamReg)
		} else if node.RestParameter.Pattern != nil {
			funcCompiler.currentSymbolTable.Define("__rest__", restParamReg)
		}
		funcCompiler.regAlloc.Pin(restParamReg)
		// Rest parameter collection is handled at runtime during function call
	}

	// Compile body
	bodyReg := funcCompiler.regAlloc.Alloc()
	// Mark if compiling function body BlockStatement
	if _, isBlock := node.Body.(*parser.BlockStatement); isBlock {
		funcCompiler.isCompilingFunctionBody = true
	}
	_, bodyErr := funcCompiler.compileNode(node.Body, bodyReg)
	funcCompiler.isCompilingFunctionBody = false
	if bodyErr != nil {
		funcCompiler.addError(node.Body, "error compiling arrow function body")
	}

	// Emit return
	funcCompiler.emitOpCode(vm.OpReturn, node.Token.Line)
	funcCompiler.emitByte(byte(bodyReg))

	// Create function value with custom name
	// Arity: total parameters excluding 'this' (for VM register allocation)
	// Length: params before first default (for ECMAScript function.length property)
	arity := 0
	length := 0
	seenDefault := false
	for _, param := range node.Parameters {
		if param.IsThis {
			continue
		}
		arity++
		if !seenDefault && param.DefaultValue == nil {
			length++
		} else {
			seenDefault = true
		}
	}

	functionChunk := funcCompiler.chunk
	freeSymbols := funcCompiler.freeSymbols
	regSize := funcCompiler.regAlloc.MaxRegs()
	functionChunk.NumSpillSlots = int(funcCompiler.nextSpillSlot) // Set spill slots needed

	funcValue := vm.NewFunction(arity, length, len(freeSymbols), int(regSize), node.RestParameter != nil, nameHint, functionChunk, false, node.IsAsync, true)
	constIdx := c.chunk.AddConstant(funcValue)

	return constIdx, freeSymbols, nil
}

func (c *Compiler) compileArrayLiteral(node *parser.ArrayLiteral, hint Register) (Register, errors.PaseratiError) {
	elementCount := len(node.Elements)
	// Normalize elisions (holes) to explicit undefined literals so runtime sees correct length and values
	for i, elem := range node.Elements {
		if elem == nil {
			node.Elements[i] = &parser.UndefinedLiteral{Token: parser.GetTokenFromNode(node)}
		}
	}
	if elementCount > 65535 { // Check against total element processing (16-bit limit)
		return BadRegister, NewCompileError(node, "array literal exceeds maximum size of 65535 elements")
	}

	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Check if any elements are spread elements
	hasSpread := false
	for _, elem := range node.Elements {
		if _, isSpread := elem.(*parser.SpreadElement); isSpread {
			hasSpread = true
			break
		}
	}

	// If no spread elements, use the original simple implementation
	if !hasSpread {
		return c.compileArrayLiteralSimple(node, hint, tempRegs)
	}

	// Handle array literal with spread elements
	return c.compileArrayLiteralWithSpread(node, hint, tempRegs)
}

// Original implementation for arrays without spread
func (c *Compiler) compileArrayLiteralSimple(node *parser.ArrayLiteral, hint Register, tempRegs []Register) (Register, errors.PaseratiError) {
	elementCount := len(node.Elements)
	// debug disabled

	// Dynamic chunking based on available registers.
	// We reserve half of available registers for element computation and other temps.
	available := c.regAlloc.AvailableTotal()
	if available < 0 {
		available = 0
	}
	maxChunkByAvail := available / 2
	// Cap chunk size to 32 for good locality.
	// Determine if we should use chunking path (large literal or tight registers)
	useChunking := elementCount > 32 || elementCount > maxChunkByAvail

	line := node.Token.Line

	if elementCount == 0 {
		// Empty array via OpMakeArray
		c.emitOpCode(vm.OpMakeArray, line)
		c.emitByte(byte(hint))
		c.emitByte(0)
		c.emitByte(0)
		return hint, nil
	}

	if !useChunking {
		// Fast path: try to allocate contiguous block for OpMakeArray
		firstTargetReg, ok := c.regAlloc.TryAllocContiguous(elementCount)
		if !ok {
			// Fall back to chunking if we can't get a contiguous block
			useChunking = true
		} else {
			// Compile elements directly into contiguous positions
			for i, elem := range node.Elements {
				targetReg := firstTargetReg + Register(i)
				if _, err := c.compileNode(elem, targetReg); err != nil {
					// Free already allocated registers on error
					for j := 0; j < elementCount; j++ {
						c.regAlloc.Free(firstTargetReg + Register(j))
					}
					return BadRegister, err
				}
			}
			c.emitOpCode(vm.OpMakeArray, line)
			c.emitByte(byte(hint))
			c.emitByte(byte(firstTargetReg))
			c.emitByte(byte(elementCount))
			// Free the contiguous block
			for i := 0; i < elementCount; i++ {
				c.regAlloc.Free(firstTargetReg + Register(i))
			}
			return hint, nil
		}
	}

	// Large literal path: allocate and copy in chunks to avoid register blowup
	// 1) Pre-allocate array to full length
	c.emitOpCode(vm.OpAllocArray, line)
	c.emitByte(byte(hint))
	c.emitUint16(uint16(elementCount))

	// 2) Emit in chunks
	offset := 0
	for offset < elementCount {
		// Recompute availability for each chunk
		available = c.regAlloc.AvailableTotal()
		maxChunkByAvail = available / 2
		if maxChunkByAvail < 1 {
			maxChunkByAvail = 1
		}
		remaining := elementCount - offset
		// Hard caps: 32, availability, and maximum contiguous currently possible
		maxContig := c.regAlloc.MaxContiguousAvailable()
		if maxContig < 1 {
			return BadRegister, NewCompileError(node, "array literal: no registers available for chunking")
		}
		n := remaining
		if n > 32 {
			n = 32
		}
		if n > maxChunkByAvail {
			n = maxChunkByAvail
		}
		if n > maxContig {
			n = maxContig
		}
		if n < 1 {
			n = 1
		}

		// Try to allocate a contiguous block for this chunk
		startReg, ok := c.regAlloc.TryAllocContiguous(n)
		if !ok {
			// If contiguous allocation fails, fall back to one-by-one insertion
			for i := 0; i < n; i++ {
				elemReg := c.regAlloc.Alloc()
				if _, err := c.compileNode(node.Elements[offset+i], elemReg); err != nil {
					c.regAlloc.Free(elemReg)
					return BadRegister, err
				}
				// Emit OpSetIndex to set array[offset+i] = element
				indexReg := c.regAlloc.Alloc()
				c.emitLoadNewConstant(indexReg, vm.Number(float64(offset+i)), line)
				c.emitOpCode(vm.OpSetIndex, line)
				c.emitByte(byte(hint))     // array
				c.emitByte(byte(indexReg)) // index
				c.emitByte(byte(elemReg))  // value
				c.regAlloc.Free(indexReg)
				c.regAlloc.Free(elemReg)
			}
			offset += n
			continue
		}
		// Compile chunk elements into the allocated registers
		for i := 0; i < n; i++ {
			if _, err := c.compileNode(node.Elements[offset+i], startReg+Register(i)); err != nil {
				// Free already allocated regs before returning
				for j := 0; j <= i; j++ {
					c.regAlloc.Free(startReg + Register(j))
				}
				return BadRegister, err
			}
		}
		// Copy chunk into array at current offset
		c.emitOpCode(vm.OpArrayCopy, line)
		c.emitByte(byte(hint))
		c.emitUint16(uint16(offset))
		c.emitByte(byte(startReg))
		c.emitByte(byte(n))
		// Free the temporary registers for this chunk immediately
		for i := 0; i < n; i++ {
			c.regAlloc.Free(startReg + Register(i))
		}
		offset += n
	}

	return hint, nil
}

// New implementation for arrays with spread elements
// NOTE: tempRegs parameter is kept for API compatibility but no longer used - registers are freed eagerly
func (c *Compiler) compileArrayLiteralWithSpread(node *parser.ArrayLiteral, hint Register, tempRegs []Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// Start with an empty array
	c.emitOpCode(vm.OpMakeArray, line)
	c.emitByte(byte(hint)) // DestReg: result array
	c.emitByte(0)          // StartReg: unused for empty array
	c.emitByte(0)          // Count: 0 elements

	// Process each element, adding to the result array
	// Free registers eagerly to avoid exhaustion with large arrays
	for _, elem := range node.Elements {
		switch e := elem.(type) {
		case *parser.SpreadElement:
			// Compile the spread expression (should be an array)
			spreadReg := c.regAlloc.Alloc()
			_, err := c.compileNode(e.Argument, spreadReg)
			if err != nil {
				c.regAlloc.Free(spreadReg)
				return BadRegister, err
			}

			// Use OpArraySpread to append all elements from spreadReg to hint
			c.emitOpCode(vm.OpArraySpread, line)
			c.emitByte(byte(hint))      // DestReg: result array (modified in place)
			c.emitByte(byte(spreadReg)) // SrcReg: array to spread

			// Free spreadReg immediately
			c.regAlloc.Free(spreadReg)

		default:
			// Regular element: compile and add to array
			elemReg := c.regAlloc.Alloc()
			_, err := c.compileNode(elem, elemReg)
			if err != nil {
				c.regAlloc.Free(elemReg)
				return BadRegister, err
			}

			// Create a temporary single-element array and spread it
			singleElemArrayReg := c.regAlloc.Alloc()
			c.emitOpCode(vm.OpMakeArray, line)
			c.emitByte(byte(singleElemArrayReg)) // DestReg: temporary array
			c.emitByte(byte(elemReg))            // StartReg: single element
			c.emitByte(1)                        // Count: 1 element

			// Free elemReg - no longer needed after MakeArray
			c.regAlloc.Free(elemReg)

			// Spread the single-element array into the result
			c.emitOpCode(vm.OpArraySpread, line)
			c.emitByte(byte(hint))               // DestReg: result array (modified in place)
			c.emitByte(byte(singleElemArrayReg)) // SrcReg: single-element array

			// Free singleElemArrayReg - no longer needed after spread
			c.regAlloc.Free(singleElemArrayReg)
		}
	}

	// Result array is now in hint register
	return hint, nil
}

func (c *Compiler) compileObjectLiteral(node *parser.ObjectLiteral, hint Register) (Register, errors.PaseratiError) {
	debugPrintf("Compiling Object Literal (One-by-One): %s\n", node.String())
	line := parser.GetTokenFromNode(node).Line

	// 1. Create an empty object in hint register
	c.emitMakeEmptyObject(hint, line)

	// 2. Set properties one by one
	// NOTE: We free registers eagerly after each property to avoid exhausting
	// registers for objects with many properties (e.g., 1000+ properties in minified code)
	for _, prop := range node.Properties {
		// Check if this is a spread element
		if spreadElement, isSpread := prop.Key.(*parser.SpreadElement); isSpread {
			// Handle spread syntax: {...obj}
			spreadReg := c.regAlloc.Alloc()
			_, err := c.compileNode(spreadElement.Argument, spreadReg)
			if err != nil {
				c.regAlloc.Free(spreadReg)
				return BadRegister, err
			}

			// Use OpObjectSpread to copy all properties from spreadReg to hint
			c.emitOpCode(vm.OpObjectSpread, line)
			c.emitByte(byte(hint))      // DestReg: result object (modified in place)
			c.emitByte(byte(spreadReg)) // SrcReg: object to spread

			// Free spreadReg immediately - we're done with it
			c.regAlloc.Free(spreadReg)
			continue
		}

		// Regular property: compile key and value
		// Track registers to free at end of this property
		var regsToFree []Register
		freePropertyRegs := func() {
			for _, reg := range regsToFree {
				c.regAlloc.Free(reg)
			}
		}

		var keyConstIdx uint16 = 0xFFFF // Invalid index marker
		var isComputedKey bool = false
		var keyReg Register

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
		case *parser.BigIntLiteral: // Allow bigint literal keys, convert to string
			keyStr := keyNode.Value // BigIntLiteral.Value is already string (numeric part without 'n')
			keyConstIdx = c.chunk.AddConstant(vm.String(keyStr))
		case *parser.ComputedPropertyName:
			// Handle computed keys: [expr]
			// Per ECMAScript spec, we must evaluate the key and convert to property key
			// BEFORE evaluating the value expression (important for side effects)
			isComputedKey = true
			keyExprReg := c.regAlloc.Alloc()
			_, err := c.compileNode(keyNode.Expr, keyExprReg)
			if err != nil {
				c.regAlloc.Free(keyExprReg)
				return BadRegister, err
			}
			// Convert to property key immediately (calls toString() if object)
			keyReg = c.regAlloc.Alloc()
			regsToFree = append(regsToFree, keyReg) // keyReg needed until property is set
			c.emitOpCode(vm.OpToPropertyKey, line)
			c.emitByte(byte(keyReg))
			c.emitByte(byte(keyExprReg))
			c.regAlloc.Free(keyExprReg) // keyExprReg no longer needed after ToPropertyKey
		default:
			// Handle other computed expressions directly (like InfixExpression)
			isComputedKey = true
			keyExprReg := c.regAlloc.Alloc()
			_, err := c.compileNode(keyNode, keyExprReg)
			if err != nil {
				c.regAlloc.Free(keyExprReg)
				return BadRegister, err
			}
			// Convert to property key immediately (calls toString() if object)
			keyReg = c.regAlloc.Alloc()
			regsToFree = append(regsToFree, keyReg) // keyReg needed until property is set
			c.emitOpCode(vm.OpToPropertyKey, line)
			c.emitByte(byte(keyReg))
			c.emitByte(byte(keyExprReg))
			c.regAlloc.Free(keyExprReg) // keyExprReg no longer needed after ToPropertyKey
		}

		// Handle MethodDefinition (getters/setters) specially
		if methodDef, isMethodDef := prop.Value.(*parser.MethodDefinition); isMethodDef {
			// Compile the function value
			valueReg := c.regAlloc.Alloc()
			regsToFree = append(regsToFree, valueReg)
			_, err := c.compileNode(methodDef.Value, valueReg)
			if err != nil {
				freePropertyRegs()
				return BadRegister, err
			}

			// Handle accessor properties (getters/setters)
			if methodDef.Kind == "getter" || methodDef.Kind == "setter" {
				if isComputedKey {
					// Computed accessor name - use OpDefineAccessorDynamic
					var getterReg, setterReg Register
					if methodDef.Kind == "getter" {
						getterReg = valueReg
						setterReg = c.regAlloc.Alloc()
						regsToFree = append(regsToFree, setterReg)
						c.emitLoadUndefined(setterReg, line)
					} else { // setter
						setterReg = valueReg
						getterReg = c.regAlloc.Alloc()
						regsToFree = append(regsToFree, getterReg)
						c.emitLoadUndefined(getterReg, line)
					}
					c.emitOpCode(vm.OpDefineAccessorDynamic, line)
					c.emitByte(byte(hint))      // Object register
					c.emitByte(byte(getterReg)) // Getter register
					c.emitByte(byte(setterReg)) // Setter register
					c.emitByte(byte(keyReg))    // Name register (computed at runtime)
					debugPrintf("--- OL MethodDefinition: Defined dynamic %s accessor\n", methodDef.Kind)
					freePropertyRegs()
					continue
				}
			} else if isComputedKey {
				// Computed regular method name in object literal
				// Use OpDefineMethodComputedEnumerable to set [[HomeObject]] and make enumerable
				// (object literal methods are enumerable, and need [[HomeObject]] for super)
				c.emitDefineMethodComputedEnumerable(hint, valueReg, keyReg, line)
				freePropertyRegs()
				continue
			}

			// Get the property name for the getter/setter (static keys only from here)
			var propName string

			// Extract property name from static key
			switch keyNode := prop.Key.(type) {
			case *parser.Identifier:
				propName = keyNode.Value
			case *parser.StringLiteral:
				propName = keyNode.Value
			case *parser.NumberLiteral:
				propName = keyNode.TokenLiteral()
			default:
				freePropertyRegs()
				return BadRegister, NewCompileError(prop.Key, "unsupported key type for getter/setter")
			}

			// Handle accessor properties (getters/setters)
			if methodDef.Kind == "getter" || methodDef.Kind == "setter" {
				// Use OpDefineAccessor for proper ECMAScript accessor properties
				var getterReg, setterReg Register
				if methodDef.Kind == "getter" {
					getterReg = valueReg
					setterReg = c.regAlloc.Alloc()
					regsToFree = append(regsToFree, setterReg)
					c.emitLoadUndefined(setterReg, line)
				} else { // setter
					setterReg = valueReg
					getterReg = c.regAlloc.Alloc()
					regsToFree = append(regsToFree, getterReg)
					c.emitLoadUndefined(getterReg, line)
				}
				nameIdx := c.chunk.AddConstant(vm.String(propName))
				c.emitDefineAccessor(hint, getterReg, setterReg, nameIdx, line)
				debugPrintf("--- OL MethodDefinition: Defined %s accessor for '%s'\n", methodDef.Kind, propName)
			} else {
				// Regular method - use OpDefineMethodEnumerable to set [[HomeObject]]
				// Object literal methods are enumerable per ECMAScript spec
				storeNameIdx := c.chunk.AddConstant(vm.String(propName))
				c.emitDefineMethodEnumerable(hint, valueReg, storeNameIdx, line)
				debugPrintf("--- OL MethodDefinition: Defined enumerable method '%s' with [[HomeObject]]\n", propName)
			}
			freePropertyRegs()
		} else {
			// Regular property value
			valueReg := c.regAlloc.Alloc()
			regsToFree = append(regsToFree, valueReg)
			_, err := c.compileNode(prop.Value, valueReg)
			if err != nil {
				freePropertyRegs()
				return BadRegister, err
			}
			debugPrintf("--- OL Value Compiled. valueReg: R%d\n", valueReg)

			// Detect if this is shorthand property syntax ({x} instead of {x: value})
			isShorthand := false
			if keyIdent, keyOk := prop.Key.(*parser.Identifier); keyOk {
				if valIdent, valOk := prop.Value.(*parser.Identifier); valOk {
					// Shorthand if same identifier (by reference) or same name
					isShorthand = (keyIdent == valIdent || keyIdent.Value == valIdent.Value)
				}
			}

			// Check for __proto__ special property (non-computed only)
			// Per Annex B.3.1, only colon syntax sets prototype; shorthand creates own property
			if !isComputedKey && !isShorthand && keyConstIdx != 0xFFFF {
				// Get the property name from constants to check if it's __proto__
				if int(keyConstIdx) < len(c.chunk.Constants) {
					keyVal := c.chunk.Constants[keyConstIdx]
					if keyVal.Type() == vm.TypeString && vm.AsString(keyVal) == "__proto__" {
						// Special handling for __proto__: value sets object's prototype
						c.emitOpCode(vm.OpSetPrototype, line)
						c.emitByte(byte(hint))     // Object register
						c.emitByte(byte(valueReg)) // Prototype value register
						debugPrintf("--- OL: Emitted OpSetPrototype for __proto__: value\n")
						freePropertyRegs()
						continue
					}
				}
			}

			// Set the property based on whether it's computed or static
			if isComputedKey {
				// Check if this is a computed method (FunctionLiteral with computed key)
				// Computed methods need [[HomeObject]] for super property access
				_, isFuncLit := prop.Value.(*parser.FunctionLiteral)
				if isFuncLit {
					// Use OpDefineMethodComputedEnumerable to set [[HomeObject]] and make enumerable
					c.emitDefineMethodComputedEnumerable(hint, valueReg, keyReg, line)
				} else {
					// Use OpSetIndex for computed keys: obj[keyReg] = valueReg
					c.emitOpCode(vm.OpSetIndex, line)
					c.emitByte(byte(hint))     // Object register
					c.emitByte(byte(keyReg))   // Key register (computed at runtime)
					c.emitByte(byte(valueReg)) // Value register
				}
			} else {
				// Use OpDefineDataProperty for static keys in object literals
				// This uses DefineOwnProperty semantics and can overwrite existing properties
				// including accessors (e.g., { get foo() {}, foo: 1 } - foo becomes data property)
				c.emitDefineDataProperty(hint, valueReg, keyConstIdx, line)
			}
			freePropertyRegs()
		}
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
	return c.compileFunctionLiteralWithStrict(node, nameHint, false)
}

// compileFunctionLiteralStrict compiles a function literal that is implicitly strict mode (e.g., class methods)
func (c *Compiler) compileFunctionLiteralStrict(node *parser.FunctionLiteral, nameHint string) (uint16, []*Symbol, errors.PaseratiError) {
	return c.compileFunctionLiteralWithStrict(node, nameHint, true)
}

// compileFunctionLiteralAsMethod compiles a function literal as a method (with [[HomeObject]])
// This is used for class methods and object methods with concise syntax.
// Methods are always strict mode and have a [[HomeObject]] for super property access.
func (c *Compiler) compileFunctionLiteralAsMethod(node *parser.FunctionLiteral, nameHint string) (uint16, []*Symbol, errors.PaseratiError) {
	return c.compileFunctionLiteralWithOptions(node, nameHint, true, true)
}

// compileFunctionLiteralWithStrict compiles a function literal with optional forced strict mode
func (c *Compiler) compileFunctionLiteralWithStrict(node *parser.FunctionLiteral, nameHint string, forceStrict bool) (uint16, []*Symbol, errors.PaseratiError) {
	return c.compileFunctionLiteralWithOptions(node, nameHint, forceStrict, false)
}

// compileFunctionLiteralWithOptions compiles a function literal with configurable options
// forceStrict: if true, function is compiled in strict mode (e.g., class methods)
// isMethod: if true, function will have [[HomeObject]] for super property access
func (c *Compiler) compileFunctionLiteralWithOptions(node *parser.FunctionLiteral, nameHint string, forceStrict bool, isMethod bool) (uint16, []*Symbol, errors.PaseratiError) {
	// 1. Create a new Compiler instance for the function body, linked to the current one
	functionCompiler := newFunctionCompiler(c) // <<< Keep this instance variable

	// 1.4. Set method compilation flag if this is a method (has [[HomeObject]])
	functionCompiler.isMethodCompilation = isMethod

	// 1.5. Set strict mode if forced (e.g., class methods are always strict per ECMAScript spec)
	if forceStrict {
		functionCompiler.chunk.IsStrict = true
		debugPrintf("// [compileFunctionLiteral] Forced strict mode (class method)\n")
	}

	// 1.5b. Check for 'use strict' directive in function body
	if hasStrictDirective(node.Body) {
		functionCompiler.chunk.IsStrict = true
		debugPrintf("// [compileFunctionLiteral] Detected 'use strict' directive in function body\n")
	}

	// 1.6. Strict mode validation: cannot use 'eval' or 'arguments' as function names
	if functionCompiler.chunk.IsStrict && node.Name != nil {
		if node.Name.Value == "eval" || node.Name.Value == "arguments" {
			functionCompiler.addError(node.Name, fmt.Sprintf("SyntaxError: Function name '%s' is not allowed in strict mode", node.Name.Value))
		}
		// Strict mode validation: FutureReservedWords cannot be used as function names
		if isFutureReservedWord(node.Name.Value) {
			functionCompiler.addError(node.Name, fmt.Sprintf("SyntaxError: Unexpected strict mode reserved word '%s'", node.Name.Value))
		}
	}

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
	functionCompiler.regAlloc.functionName = determinedFuncName
	// --- End Set Name ---

	// --- Set function context flags ---
	functionCompiler.isAsync = node.IsAsync
	functionCompiler.isGenerator = node.IsGenerator
	// --- End function context ---

	// --- NEW: Handle function name binding for named function expressions ---
	// For named function expressions like: let f = function g() { g(); }
	// The name 'g' should be accessible only inside the function and refer to the function itself
	//
	// We need to distinguish:
	// - Function expression: `let f = function g() {}` - g is inner binding
	// - Function declaration: `function g() {}` - g is outer binding (passed as nameHint)
	//
	// If node.Name matches nameHint, it's a declaration (name already in outer scope)
	// If node.Name differs from nameHint or nameHint is empty, it's a named expression
	var funcNameForInnerBinding string
	var needsInnerNameBinding bool
	if node.Name != nil {
		// Check if this is a named function expression (not a declaration)
		if nameHint == "" || nameHint != node.Name.Value {
			// This is a named function expression - create inner binding
			funcNameForInnerBinding = node.Name.Value
			needsInnerNameBinding = true
		}
		// If nameHint == node.Name.Value, it's a declaration, no inner binding needed
	}
	// For recursive anonymous functions (let f = function() { f(); }), the name
	// resolves from outer scope naturally, so we don't need special handling
	// --- END NEW ---

	debugPrintf("// [Compiling Function Literal] %s\n", determinedFuncName)

	// 2. Define parameters in the function compiler's *enclosed* scope
	// Track seen parameter names for duplicate detection in strict mode
	seenParams := make(map[string]bool)

	for _, param := range node.Parameters {
		// Skip 'this' parameters - they don't have names and don't get compiled as regular parameters
		if param.IsThis {
			continue
		}
		// Skip destructuring parameters if parser didn't transform them
		// The desugared declaration statements in the function body will handle binding
		if param.IsDestructuring {
			// Parser should have transformed these, but if not, skip them
			// The destructuring pattern will be handled as statements in the function body
			continue
		}
		// param.Name should never be nil for non-destructuring parameters
		if param.Name == nil {
			// Create a temporary identifier node for error reporting
			tempNode := &parser.Identifier{Token: param.Token, Value: "<parameter>"}
			functionCompiler.addError(tempNode, "parameter has nil Name but IsDestructuring is false")
			continue
		}

		// Strict mode validation: cannot use 'eval' or 'arguments' as parameter names
		if functionCompiler.chunk.IsStrict && (param.Name.Value == "eval" || param.Name.Value == "arguments") {
			functionCompiler.addError(param.Name, fmt.Sprintf("SyntaxError: Strict mode function may not have parameter named '%s'", param.Name.Value))
			// Continue defining it to avoid cascading errors
		}

		// Strict mode validation: FutureReservedWords cannot be used as parameter names
		if functionCompiler.chunk.IsStrict && isFutureReservedWord(param.Name.Value) {
			functionCompiler.addError(param.Name, fmt.Sprintf("SyntaxError: Unexpected strict mode reserved word '%s'", param.Name.Value))
		}

		// Strict mode validation: duplicate parameter names are forbidden
		if functionCompiler.chunk.IsStrict {
			if seenParams[param.Name.Value] {
				functionCompiler.addError(param.Name, fmt.Sprintf("SyntaxError: Duplicate parameter name '%s' not allowed in strict mode", param.Name.Value))
			}
			seenParams[param.Name.Value] = true
		}

		reg := functionCompiler.regAlloc.Alloc()
		functionCompiler.currentSymbolTable.Define(param.Name.Value, reg)
		// Track parameter names for var hoisting (var x; should not reset parameter x)
		functionCompiler.parameterNames[param.Name.Value] = true
		// Pin the register since parameters can be captured by inner functions
		functionCompiler.regAlloc.Pin(reg)
		debugPrintf("// [Compiling Function Literal] %s: Parameter %s defined in R%d\n", determinedFuncName, param.Name.Value, reg)
	}

	// 3. Handle default parameters
	// Build parameter list for TDZ checking (excluding 'this' and destructuring params)
	functionCompiler.parameterList = make([]string, 0, len(node.Parameters))
	for _, param := range node.Parameters {
		if param.IsThis || param.IsDestructuring || param.Name == nil {
			continue
		}
		functionCompiler.parameterList = append(functionCompiler.parameterList, param.Name.Value)
	}

	paramIdx := 0
	for _, param := range node.Parameters {
		// Skip 'this' parameters - they don't have default values and aren't in the symbol table
		if param.IsThis {
			continue
		}
		// Skip destructuring parameters - defaults are handled in the desugared declarations
		if param.IsDestructuring {
			continue
		}
		if param.DefaultValue != nil {
			// param.Name should not be nil here (we checked IsDestructuring above)
			if param.Name == nil {
				tempNode := &parser.Identifier{Token: param.Token, Value: "<parameter>"}
				functionCompiler.addError(tempNode, "non-destructuring parameter has nil Name")
				paramIdx++
				continue
			}
			// Get the parameter's register
			symbol, _, exists := functionCompiler.currentSymbolTable.Resolve(param.Name.Value)
			if !exists {
				// This should not happen if parameter definition worked correctly
				functionCompiler.addError(param.Name, fmt.Sprintf("parameter %s not found in symbol table", param.Name.Value))
				paramIdx++
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
			// Set TDZ tracking: parameters at index >= paramIdx are in TDZ
			defaultValueReg := functionCompiler.regAlloc.Alloc()
			var beforeMaxReg Register
			if debugRegAlloc {
				beforeMaxReg = functionCompiler.regAlloc.maxReg
				fmt.Printf("// [PARAM_DEBUG] Before compileNode for param %s: maxReg=%d\\n", param.Name.Value, beforeMaxReg)
			}
			functionCompiler.currentDefaultParamIndex = paramIdx
			functionCompiler.inDefaultParamScope = true
			_, err := functionCompiler.compileNode(param.DefaultValue, defaultValueReg)
			functionCompiler.inDefaultParamScope = false
			functionCompiler.currentDefaultParamIndex = -1
			if debugRegAlloc {
				afterMaxReg := functionCompiler.regAlloc.maxReg
				fmt.Printf("// [PARAM_DEBUG] After compileNode for param %s: maxReg was %d, now %d\\n", param.Name.Value, beforeMaxReg, afterMaxReg)
			}
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
		paramIdx++ // Increment for all non-skipped parameters
	}

	// 3.5. Handle destructuring parameters
	// For each destructuring parameter, allocate a register and compile the destructuring code
	// This must happen BEFORE OpInitYield (for generators) so that destructuring errors
	// are thrown during construction, not during the first .next() call
	for i, param := range node.Parameters {
		if debugCompiler {
			fmt.Printf("// [Compiler] Checking param %d: IsThis=%v, IsDestructuring=%v, Pattern=%v\n",
				i, param.IsThis, param.IsDestructuring, param.Pattern != nil)
		}
		if param.IsThis || !param.IsDestructuring || param.Pattern == nil {
			continue
		}

		// Allocate a register for this destructuring parameter
		// The VM will place the argument value in this register during function call
		paramReg := functionCompiler.regAlloc.Alloc()
		debugPrintf("// [Compiler] Destructuring parameter %d allocated R%d\n", i, paramReg)

		// Handle default value for destructuring parameter
		// If paramReg is undefined, use the default value instead
		if param.DefaultValue != nil {
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
			// Calculate TDZ index: count named params before position i
			tdzIndex := 0
			for j := 0; j < i; j++ {
				p := node.Parameters[j]
				if !p.IsThis && !p.IsDestructuring && p.Name != nil {
					tdzIndex++
				}
			}
			defaultValueReg := functionCompiler.regAlloc.Alloc()
			functionCompiler.currentDefaultParamIndex = tdzIndex
			functionCompiler.inDefaultParamScope = true
			_, err := functionCompiler.compileNode(param.DefaultValue, defaultValueReg)
			functionCompiler.inDefaultParamScope = false
			functionCompiler.currentDefaultParamIndex = -1
			if err != nil {
				functionCompiler.addError(param.DefaultValue, fmt.Sprintf("error compiling default value for destructuring parameter: %v", err))
			} else {
				// Move the default value to the parameter register
				if defaultValueReg != paramReg {
					functionCompiler.emitMove(paramReg, defaultValueReg, param.Token.Line)
				}
			}
			functionCompiler.regAlloc.Free(defaultValueReg)

			// Patch the jump to come here (end of default value assignment)
			functionCompiler.patchJump(jumpIfDefinedPos)
		}

		// Compile the destructuring pattern
		// The destructuring will extract values from paramReg and bind them to local variables
		switch pattern := param.Pattern.(type) {
		case *parser.ArrayParameterPattern:
			err := functionCompiler.compileNestedArrayParameterPattern(pattern, paramReg, false, param.Token.Line)
			if err != nil {
				functionCompiler.addError(param, fmt.Sprintf("error compiling parameter destructuring: %v", err))
			}
		case *parser.ObjectParameterPattern:
			err := functionCompiler.compileNestedObjectParameterPattern(pattern, paramReg, false, param.Token.Line)
			if err != nil {
				functionCompiler.addError(param, fmt.Sprintf("error compiling parameter destructuring: %v", err))
			}
		default:
			functionCompiler.addError(param, "unsupported parameter pattern type")
		}
	}

	// 4. Handle rest parameter (if present)
	var restParamReg Register
	var restParamPattern parser.Expression // Save pattern for later destructuring
	if node.RestParameter != nil {
		// Allocate register for the rest parameter array
		// The VM will populate this register with the rest arguments array
		restParamReg = functionCompiler.regAlloc.Alloc()

		// Check if it's a simple identifier or destructuring pattern
		if node.RestParameter.Name != nil {
			// Simple rest parameter like ...args
			functionCompiler.currentSymbolTable.Define(node.RestParameter.Name.Value, restParamReg)
			// Track rest parameter name for var hoisting
			functionCompiler.parameterNames[node.RestParameter.Name.Value] = true
			// Pin the register since rest parameters can be captured by inner functions
			functionCompiler.regAlloc.Pin(restParamReg)
			debugPrintf("// [Compiler] Rest parameter '%s' defined in R%d\n", node.RestParameter.Name.Value, restParamReg)
		} else if node.RestParameter.Pattern != nil {
			// Destructuring rest parameter like ...[x, y] or ...{a, b}
			// Save the pattern for generating destructuring code after function prologue
			restParamPattern = node.RestParameter.Pattern
			debugPrintf("// [Compiler] Rest parameter with destructuring pattern in R%d\n", restParamReg)
		}
	}

	// 4.5. Handle named function expression binding
	// For named function expressions like: function g() { g(); }
	// The name 'g' should be accessible inside and refer to the closure itself
	if needsInnerNameBinding {
		// Allocate a register for the function name binding
		nameBindingReg := functionCompiler.regAlloc.Alloc()
		functionCompiler.currentSymbolTable.Define(funcNameForInnerBinding, nameBindingReg)
		functionCompiler.regAlloc.Pin(nameBindingReg) // Pin since it can be captured

		// No bytecode needs to be emitted here - the VM will initialize this register
		// when the function is called (see call.go prepareCall)
		debugPrintf("// [Compiler] Function name binding '%s' allocated in R%d (will be initialized by VM)\n",
			funcNameForInnerBinding, nameBindingReg)
	}

	// 4.6. Generate destructuring code for rest parameter pattern (if needed)
	// This must happen BEFORE compiling the function body so the destructured variables are available
	if restParamPattern != nil {
		// Generate destructuring code based on pattern type
		switch pattern := restParamPattern.(type) {
		case *parser.ArrayParameterPattern:
			// Convert to destructuring and compile it
			err := functionCompiler.compileNestedArrayParameterPattern(pattern, restParamReg, false, node.Token.Line)
			if err != nil {
				functionCompiler.addError(node.RestParameter, fmt.Sprintf("error compiling rest parameter destructuring: %v", err))
			}
		case *parser.ObjectParameterPattern:
			// Convert to destructuring and compile it
			err := functionCompiler.compileNestedObjectParameterPattern(pattern, restParamReg, false, node.Token.Line)
			if err != nil {
				functionCompiler.addError(node.RestParameter, fmt.Sprintf("error compiling rest parameter destructuring: %v", err))
			}
		default:
			functionCompiler.addError(node.RestParameter, "unsupported rest parameter pattern type")
		}
	}

	// 4.5. For generators, emit OpInitYield AFTER compiling desugared parameter declarations
	// The parser desugars destructuring parameters into const declarations at the start of the body
	// We need to compile those first, then emit OpInitYield, then compile the rest of the body
	if node.IsGenerator {
		debugPrintf("// [Generator] Compiling with OpInitYield after desugared parameters\n")

		// Compile the body specially for generators to insert OpInitYield at the right place
		bodyReg := functionCompiler.regAlloc.Alloc()
		functionCompiler.isCompilingFunctionBody = true

		// Get the body as a block statement (it's always a BlockStatement for function literals)
		blockBody := node.Body
		if blockBody != nil {
			// Compile desugared parameter declarations first (they're at the start of the block)
			// The parser transforms destructuring parameters into regular parameters named __destructured_param_N
			// and adds ArrayDestructuringDeclaration or ObjectDestructuringDeclaration statements at the start of the body
			// We need to compile these BEFORE OpInitYield so errors throw during construction
			desugarCount := 0
			for i, stmt := range blockBody.Statements {
				if debugCompiler {
					fmt.Printf("// [Generator] Checking statement %d: type=%T\n", i, stmt)
				}
				// Check if this is a desugared parameter declaration
				isDesugared := false

				if arrayDecl, ok := stmt.(*parser.ArrayDestructuringDeclaration); ok {
					// Check if the value is an identifier matching __destructured_param_*
					if ident, ok := arrayDecl.Value.(*parser.Identifier); ok {
						if len(ident.Value) >= 21 && ident.Value[:21] == "__destructured_param_" {
							isDesugared = true
						}
					}
				} else if objDecl, ok := stmt.(*parser.ObjectDestructuringDeclaration); ok {
					// Check if the value is an identifier matching __destructured_param_*
					if ident, ok := objDecl.Value.(*parser.Identifier); ok {
						if debugCompiler {
							fmt.Printf("// [Generator]   ObjectDestructuringDeclaration.Value = %s\n", ident.Value)
						}
						if len(ident.Value) >= 21 && ident.Value[:21] == "__destructured_param_" {
							isDesugared = true
						}
					} else if debugCompiler {
						fmt.Printf("// [Generator]   ObjectDestructuringDeclaration.Value is not Identifier: %T\n", objDecl.Value)
					}
				}

				if !isDesugared {
					break // Stop when we hit a non-desugared statement
				}

				// Compile this desugared parameter declaration
				stmtReg := functionCompiler.regAlloc.Alloc()
				functionCompiler.compileNode(stmt, stmtReg)
				functionCompiler.regAlloc.Free(stmtReg)
				desugarCount++

				debugPrintf("// [Generator] Compiled desugared parameter declaration %d\n", i)
			}

			// NOW emit OpInitYield after desugared parameters
			functionCompiler.emitOpCode(vm.OpInitYield, node.Body.Token.Line)
			debugPrintf("// [Generator] Emitted OpInitYield after %d desugared parameter declarations\n", desugarCount)

			// Compile remaining statements (the actual body)
			for i := desugarCount; i < len(blockBody.Statements); i++ {
				stmtReg := functionCompiler.regAlloc.Alloc()
				functionCompiler.compileNode(blockBody.Statements[i], stmtReg)
				functionCompiler.regAlloc.Free(stmtReg)
			}
		} else {
			// Non-block body (arrow function expression) - just emit OpInitYield first
			functionCompiler.emitOpCode(vm.OpInitYield, node.Body.Token.Line)
			functionCompiler.compileNode(node.Body, bodyReg)
		}

		functionCompiler.isCompilingFunctionBody = false
		functionCompiler.regAlloc.Free(bodyReg)
	} else {
		// Non-generator: compile body normally
		bodyReg := functionCompiler.regAlloc.Alloc()
		functionCompiler.isCompilingFunctionBody = true
		functionCompiler.compileNode(node.Body, bodyReg)
		functionCompiler.isCompilingFunctionBody = false
		functionCompiler.regAlloc.Free(bodyReg)
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
	functionChunk.NumSpillSlots = int(functionCompiler.nextSpillSlot) // Set spill slots needed

	// Generate scope descriptor if this function contains direct eval
	if functionCompiler.hasDirectEval {
		functionChunk.ScopeDesc = functionCompiler.generateScopeDescriptor()
		debugPrintf("// [Compiler] Function '%s' has direct eval, generated scope descriptor with %d locals\n",
			determinedFuncName, len(functionChunk.ScopeDesc.LocalNames))
	}

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

	// DEBUG: Print RegisterSize for all functions
	if debugRegAlloc {
		fmt.Printf("[REGSIZE] Function '%s' (gen=%v) has RegisterSize=%d (maxReg=%d)\n", funcName, node.IsGenerator, regSize, functionCompiler.regAlloc.maxReg)
	}

	// DEBUG: Disassemble generator functions
	if debugCompiledCode && node.IsGenerator {
		fmt.Printf("\n=== GENERATOR BYTECODE: %s ===\n", funcName)
		fmt.Print(functionChunk.DisassembleChunk(funcName))
		fmt.Printf("=== END GENERATOR ===\n\n")
	}

	// 8. Add the function object to the *outer* compiler's constant pool.
	// Arity: total parameters excluding 'this' (for VM register allocation)
	// Length: params before first default (for ECMAScript function.length property)
	arity := 0
	length := 0
	seenDefault := false
	for _, param := range node.Parameters {
		if param.IsThis {
			continue
		}
		arity++
		if !seenDefault && param.DefaultValue == nil {
			length++
		} else {
			seenDefault = true
		}
	}
	funcValue := vm.NewFunction(arity, length, len(freeSymbols), int(regSize), node.RestParameter != nil, funcName, functionChunk, node.IsGenerator, node.IsAsync, false) // isArrowFunction = false for regular functions

	// Set the name binding register if this is a named function expression
	if needsInnerNameBinding {
		// Find the register we allocated for the name binding
		if symbol, _, found := functionCompiler.currentSymbolTable.Resolve(funcNameForInnerBinding); found {
			funcObj := funcValue.AsFunction()
			funcObj.NameBindingRegister = int(symbol.Register)
			debugPrintf("// [Compiler] Function '%s' has name binding in R%d\n", funcName, symbol.Register)
		}
	}

	constIdx := c.chunk.AddConstant(funcValue)

	// <<< REMOVE OpClosure EMISSION FROM HERE (should already be removed) >>>

	// --- Return the constant index, the free symbols, and nil error ---
	// Accumulated errors are in c.errors.
	return constIdx, freeSymbols, nil // <<< MODIFY return statement
}

// hasStrictDirective checks if a block statement begins with a directive prologue containing 'use strict'.
// Per ECMAScript spec, a directive prologue is the longest sequence of ExpressionStatements
// at the start of a function body where each is a string literal. Any "use strict" in this
// sequence enables strict mode.
func hasStrictDirective(body *parser.BlockStatement) bool {
	if body == nil || len(body.Statements) == 0 {
		return false
	}

	// Iterate through the directive prologue (consecutive string literal ExpressionStatements)
	for _, stmt := range body.Statements {
		exprStmt, ok := stmt.(*parser.ExpressionStatement)
		if !ok {
			// Not an ExpressionStatement - directive prologue ends
			break
		}
		strLit, ok := exprStmt.Expression.(*parser.StringLiteral)
		if !ok {
			// ExpressionStatement but not a string literal - directive prologue ends
			break
		}
		// This is a directive - check if it's "use strict"
		if strLit.Value == "use strict" {
			return true
		}
		// Otherwise it's another directive (like "another directive"), continue checking
	}

	return false
}

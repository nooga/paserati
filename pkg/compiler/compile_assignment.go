package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

const debugAssignment = false // Enable debug output for assignment compilation

// WithPropertyInfo contains information about how to handle a with property access
type WithPropertyInfo struct {
	UseWithProperty bool     // True if this should use with-property resolution
	HasLocalFallback bool    // True if there's a local variable to fall back to
	LocalReg        Register // The local register to fall back to (if HasLocalFallback)
}

// shouldUseWithProperty checks if an identifier should be treated as a with property
// Returns info about whether to use with-property resolution and any local fallback
func (c *Compiler) shouldUseWithProperty(ident *parser.Identifier) (Register, bool) {
	info := c.getWithPropertyInfo(ident)
	return info.LocalReg, info.UseWithProperty
}

// getWithPropertyInfo returns detailed information about how to handle an identifier
// in a with block context - whether it needs with-property resolution and if there's
// a local variable fallback
func (c *Compiler) getWithPropertyInfo(ident *parser.Identifier) WithPropertyInfo {
	// If we're not inside a with block, don't use with property resolution
	if c.withBlockDepth == 0 {
		return WithPropertyInfo{UseWithProperty: false}
	}

	// Check if this identifier is a local variable in the CURRENT function scope
	symbolRef, definingTable, found := c.currentSymbolTable.Resolve(ident.Value)

	if found && !symbolRef.IsGlobal && !symbolRef.IsSpilled {
		// Check if this is an upvalue (defined in an outer function)
		// If so, we should NOT use the local fallback path
		if c.enclosing != nil && c.isDefinedInEnclosingCompiler(definingTable) {
			// It's an upvalue - use OpGetWithProperty/OpSetWithProperty (falls back to upvalue resolution)
			return WithPropertyInfo{
				UseWithProperty:  true,
				HasLocalFallback: false,
			}
		}

		// It's a local variable in the current function.
		// Only use with-property resolution if the with block is in the CURRENT function.
		// Nested functions have their own scope - their locals should NOT be shadowed by
		// an enclosing with-object from a parent function.
		if c.currentFuncWithDepth > 0 {
			// With block is in current function - use OpSetWithOrLocal
			// to check with-object first, then fall back to the local
			return WithPropertyInfo{
				UseWithProperty:  true,
				HasLocalFallback: true,
				LocalReg:         symbolRef.Register,
			}
		}
		// With block is only in parent function - local wins, don't use with-property
		return WithPropertyInfo{UseWithProperty: false}
	}

	// No local variable in current function - use OpGetWithProperty/OpSetWithProperty (falls back to globals/upvalues)
	return WithPropertyInfo{
		UseWithProperty:  true,
		HasLocalFallback: false,
	}
}

// compileAssignmentExpression compiles identifier = value OR indexExpr = value OR memberExpr = value
func (c *Compiler) compileAssignmentExpression(node *parser.AssignmentExpression, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// For class field initializers, set the flag so eval inside can detect this context
	// and forbid 'arguments' access per ES spec
	prevIsClassFieldInitializer := c.isClassFieldInitializer
	if node.IsFieldInitializer {
		c.isClassFieldInitializer = true
	}
	defer func() {
		c.isClassFieldInitializer = prevIsClassFieldInitializer
	}()

	// --- Refactored LHS Handling ---
	var currentValueReg Register // Register holding the value BEFORE the assignment/operation
	var needsStore bool = true   // Assume we need to store back by default
	type lhsInfoType int
	const (
		lhsIsIdentifier lhsInfoType = iota
		lhsIsIndexExpr
		lhsIsMemberExpr
	)
	var lhsType lhsInfoType
	var identInfo struct { // Info needed to store back to identifier
		targetReg        Register
		isUpvalue        bool
		upvalueIndex     uint16 // 16-bit to support large closures (up to 65535 upvalues)
		isGlobal         bool   // Track if this is a global variable
		globalIdx        uint16 // Direct global index instead of name constant index
		isCallerLocal    bool   // Track if this is a caller's local (for direct eval)
		callerRegIdx     int    // Caller's register index (for direct eval)
		isSpilled        bool   // Track if this is a spilled variable
		spillIndex       uint16 // Spill slot index (for spilled variables)
		isWithOrLocal    bool   // Track if this is a with-property with local fallback
		withNameConstIdx uint16 // Name constant index for with-property
		withLocalReg     Register // Local register to fall back to
		withBindingReg   Register // Register holding captured binding (for simple assignments)
	}
	var indexInfo struct { // Info needed to store back to index expr
		arrayReg Register
		indexReg Register
	}
	var memberInfo struct { // Info needed for member expr
		objectReg         Register
		nameConstIdx      uint16   // For static properties
		isComputed        bool     // True if this is a computed property
		keyReg            Register // For computed properties
		isPrivateField    bool     // True if this is a private field (#field)
		isPrivateSetter   bool     // True if this is a private setter (set #field)
		isWithProperty    bool     // True if this is a with property (use OpGetWithProperty/OpSetWithProperty)
	}

	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		// Clean up all temporary registers
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	switch lhsNode := node.Left.(type) {
	case *parser.Identifier:
		lhsType = lhsIsIdentifier

		// Strict mode validation: cannot assign to 'eval' or 'arguments'
		if c.chunk.IsStrict && (lhsNode.Value == "eval" || lhsNode.Value == "arguments") {
			c.addError(lhsNode, fmt.Sprintf("SyntaxError: Cannot assign to '%s' in strict mode", lhsNode.Value))
		}

		// Strict mode validation: cannot assign to FutureReservedWords
		if c.chunk.IsStrict && isFutureReservedWord(lhsNode.Value) {
			c.addError(lhsNode, fmt.Sprintf("SyntaxError: Unexpected strict mode reserved word '%s'", lhsNode.Value))
		}

		// First check if this is a with property (highest priority)
		withInfo := c.getWithPropertyInfo(lhsNode)
		if withInfo.UseWithProperty {
			if withInfo.HasLocalFallback {
				// With-property with local fallback - need to capture binding BEFORE RHS evaluation
				// per ECMAScript reference binding semantics
				lhsType = lhsIsIdentifier
				identInfo.isWithOrLocal = true
				identInfo.withNameConstIdx = c.chunk.AddConstant(vm.String(lhsNode.Value))
				identInfo.withLocalReg = withInfo.LocalReg

				// Capture the binding BEFORE evaluating RHS (required by ECMAScript spec)
				// This determines whether to use with-object or local at assignment start
				identInfo.withBindingReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, identInfo.withBindingReg)
				c.emitResolveWithBinding(identInfo.withBindingReg, int(identInfo.withNameConstIdx), withInfo.LocalReg, line)

				// For compound assignments, we also need the current property value
				if node.Operator != "=" {
					currentValueReg = c.regAlloc.Alloc()
					tempRegs = append(tempRegs, currentValueReg)
					c.emitGetWithOrLocal(currentValueReg, int(identInfo.withNameConstIdx), withInfo.LocalReg, line)
				} else {
					currentValueReg = nilRegister // Not needed for simple assignment
				}
			} else {
				// With-property without local fallback - still need binding capture for strict mode
				// Use same approach but with localReg=255 to indicate no local fallback
				lhsType = lhsIsIdentifier
				identInfo.isWithOrLocal = true
				identInfo.withNameConstIdx = c.chunk.AddConstant(vm.String(lhsNode.Value))
				identInfo.withLocalReg = 255 // Sentinel: no local, fall back to global

				// Capture the binding BEFORE evaluating RHS
				identInfo.withBindingReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, identInfo.withBindingReg)
				c.emitResolveWithBinding(identInfo.withBindingReg, int(identInfo.withNameConstIdx), 255, line)

				// For compound assignments, we need the current property value
				if node.Operator != "=" {
					currentValueReg = c.regAlloc.Alloc()
					tempRegs = append(tempRegs, currentValueReg)
					c.emitGetWithProperty(currentValueReg, int(identInfo.withNameConstIdx), line)
				} else {
					currentValueReg = nilRegister // Not needed for simple assignment
				}
			}
		} else {
			// Regular identifier - resolve the identifier
			symbolRef, definingTable, found := c.currentSymbolTable.Resolve(lhsNode.Value)

			// Check for const assignment - emit TypeError at runtime
			if found && symbolRef.IsConst {
				c.emitConstAssignmentError(lhsNode.Value, line)
				// Still need to evaluate RHS for side effects, but return after
				if _, err := c.compileNode(node.Value, hint); err != nil {
					return BadRegister, err
				}
				return hint, nil
			}

			// Check for immutable binding (NFE name binding)
			// In non-strict mode, assignment is silently ignored
			// In strict mode, it should throw a TypeError (but we currently just ignore)
			if found && symbolRef.IsImmutable {
				// For immutable bindings, skip the store
				// RHS is still evaluated for side effects
				needsStore = false
				// Still set up identInfo so the code flow continues properly
				identInfo.targetReg = symbolRef.Register
				identInfo.isUpvalue = false
				identInfo.isGlobal = false
				currentValueReg = identInfo.targetReg
			} else if !found {
				// Check caller scope first (for direct eval with scope access)
				if c.callerScopeDesc != nil {
					if callerRegIdx := c.resolveCallerLocal(lhsNode.Value); callerRegIdx >= 0 {
						identInfo.isCallerLocal = true
						identInfo.callerRegIdx = callerRegIdx
						// For compound assignments, we need the current value
						if node.Operator != "=" {
							currentValueReg = c.regAlloc.Alloc()
							tempRegs = append(tempRegs, currentValueReg)
							c.emitOpCode(vm.OpGetCallerLocal, line)
							c.emitByte(byte(currentValueReg))
							c.emitByte(byte(callerRegIdx))
						} else {
							currentValueReg = nilRegister // Not needed for simple assignment
						}
					} else {
						// Not in caller scope either
						// In strict mode, assignment to undeclared variable throws ReferenceError
						// But first check if this is an existing global (e.g., var/let/const at global scope)
						// Also skip for 'arguments' and 'eval' which have special handling elsewhere
						if c.chunk.IsStrict && !c.GlobalExists(lhsNode.Value) && lhsNode.Value != "arguments" && lhsNode.Value != "eval" {
							// Emit runtime error for strict mode undeclared variable assignment
							c.emitStrictUndeclaredAssignmentError(lhsNode.Value, line)
							// Still evaluate RHS for side effects
							if _, err := c.compileNode(node.Value, hint); err != nil {
								return BadRegister, err
							}
							return hint, nil
						}
						// Either non-strict mode or existing global: treat as global assignment
						identInfo.isGlobal = true
						identInfo.globalIdx = c.GetOrAssignGlobalIndex(lhsNode.Value)
						if node.Operator != "=" {
							currentValueReg = c.regAlloc.Alloc()
							tempRegs = append(tempRegs, currentValueReg)
							c.emitGetGlobal(currentValueReg, identInfo.globalIdx, line)
						} else {
							currentValueReg = nilRegister
						}
					}
				} else {
					// Variable not found in any scope
					// In strict mode, assignment to undeclared variable throws ReferenceError
					// But first check if this is an existing global (e.g., var/let/const at global scope)
					// Also skip for 'arguments' and 'eval' which have special error handling elsewhere
					if c.chunk.IsStrict && !c.GlobalExists(lhsNode.Value) && lhsNode.Value != "arguments" && lhsNode.Value != "eval" {
						// Emit runtime error for strict mode undeclared variable assignment
						c.emitStrictUndeclaredAssignmentError(lhsNode.Value, line)
						// Still evaluate RHS for side effects
						if _, err := c.compileNode(node.Value, hint); err != nil {
							return BadRegister, err
						}
						return hint, nil
					}
					// Either non-strict mode or existing global: treat as global assignment
					identInfo.isGlobal = true
					identInfo.globalIdx = c.GetOrAssignGlobalIndex(lhsNode.Value)
					// For compound assignments, we need the current value
					if node.Operator != "=" {
						currentValueReg = c.regAlloc.Alloc()
						tempRegs = append(tempRegs, currentValueReg)
						c.emitGetGlobal(currentValueReg, identInfo.globalIdx, line)
					} else {
						currentValueReg = nilRegister // Not needed for simple assignment
					}
				}
			} else if symbolRef.IsCallerLocal {
				// Caller local variable (for direct eval)
				identInfo.isCallerLocal = true
				identInfo.callerRegIdx = symbolRef.CallerLocalIndex
				// For compound assignments, we need the current value
				if node.Operator != "=" {
					currentValueReg = c.regAlloc.Alloc()
					tempRegs = append(tempRegs, currentValueReg)
					c.emitOpCode(vm.OpGetCallerLocal, line)
					c.emitByte(byte(currentValueReg))
					c.emitByte(byte(symbolRef.CallerLocalIndex))
				} else {
					currentValueReg = nilRegister // Not needed for simple assignment
				}
			} else {
				// Determine target register/upvalue index and load current value
				if symbolRef.IsGlobal {
					// Global variable
					identInfo.isGlobal = true
					identInfo.globalIdx = symbolRef.GlobalIndex
					// For compound assignments, we need the current value
					if node.Operator != "=" {
						currentValueReg = c.regAlloc.Alloc()
						tempRegs = append(tempRegs, currentValueReg)
						c.emitGetGlobal(currentValueReg, identInfo.globalIdx, line)
					} else {
						currentValueReg = nilRegister // Not needed for simple assignment
					}
				} else if definingTable == c.currentSymbolTable {
					// Local variable in current scope
					if symbolRef.IsSpilled {
						// Spilled variable - use spill slot
						identInfo.isSpilled = true
						identInfo.spillIndex = symbolRef.SpillIndex
						identInfo.isUpvalue = false
						identInfo.isGlobal = false
						// For compound assignments, we need to load the current value
						if node.Operator != "=" {
							currentValueReg = c.regAlloc.Alloc()
							tempRegs = append(tempRegs, currentValueReg)
							c.emitLoadSpill(currentValueReg, identInfo.spillIndex, line)
						} else {
							currentValueReg = nilRegister // Not needed for simple assignment
						}
					} else {
						// Regular register-allocated variable
						identInfo.targetReg = symbolRef.Register
						identInfo.isUpvalue = false
						identInfo.isGlobal = false
						currentValueReg = identInfo.targetReg // Current value is already in targetReg
					}
				} else if c.enclosing != nil && c.isDefinedInEnclosingCompiler(definingTable) {
					// Variable defined in outer function: treat as upvalue
					identInfo.isUpvalue = true
					identInfo.isGlobal = false
					identInfo.upvalueIndex = c.addFreeSymbol(node, &symbolRef)
					currentValueReg = c.regAlloc.Alloc() // Allocate temporary reg for current value
					tempRegs = append(tempRegs, currentValueReg)
					c.emitLoadFree(currentValueReg, identInfo.upvalueIndex, line)
				} else {
					// Variable in outer block scope of same function (or at top level)
					if symbolRef.IsSpilled {
						// Spilled variable - use spill slot
						identInfo.isSpilled = true
						identInfo.spillIndex = symbolRef.SpillIndex
						identInfo.isUpvalue = false
						identInfo.isGlobal = false
						// For compound assignments, we need to load the current value
						if node.Operator != "=" {
							currentValueReg = c.regAlloc.Alloc()
							tempRegs = append(tempRegs, currentValueReg)
							c.emitLoadSpill(currentValueReg, identInfo.spillIndex, line)
						} else {
							currentValueReg = nilRegister // Not needed for simple assignment
						}
					} else {
						// Regular register-allocated variable: access directly via register
						identInfo.targetReg = symbolRef.Register
						identInfo.isUpvalue = false
						identInfo.isGlobal = false
						currentValueReg = identInfo.targetReg // Current value is already in targetReg
					}
				}
			}
		}

	case *parser.IndexExpression:
		// Check for super indexed assignment (super[expr] = value or super[expr] op= value)
		if _, isSuper := lhsNode.Left.(*parser.SuperExpression); isSuper {
			// Super indexed assignment requires special handling with OpSetSuperComputedWithBase
			// Property lookup uses super base, but assignment is on 'this'
			// IMPORTANT: Per ECMAScript spec, super base must be captured BEFORE evaluating the key
			// This is because ToPropertyKey (which calls toString) happens in PutValue AFTER
			// the super reference is created

			// Check if this is a compound assignment
			isCompound := node.Operator != "="

			// Step 1: Capture super base FIRST (before key evaluation)
			baseReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, baseReg)
			c.chunk.WriteOpCode(vm.OpLoadSuper, line)
			c.chunk.EmitByte(byte(baseReg))

			// Step 2: Compile the index expression (may call toString() which could mutate prototype)
			keyReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, keyReg)
			_, err := c.compileNode(lhsNode.Index, keyReg)
			if err != nil {
				return BadRegister, err
			}

			var currentValueReg Register
			if isCompound {
				// Step 3: For compound assignment, read current value using OpGetSuperComputed
				currentValueReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, currentValueReg)
				c.chunk.WriteOpCode(vm.OpGetSuperComputed, line)
				c.chunk.EmitByte(byte(currentValueReg)) // destination
				c.chunk.EmitByte(byte(keyReg))          // key
			}

			// Step 4: Compile the RHS value
			rhsReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, rhsReg)
			_, err = c.compileNode(node.Value, rhsReg)
			if err != nil {
				return BadRegister, err
			}

			// Step 5: Compute final value
			valueReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, valueReg)

			if isCompound {
				// Arithmetic/bitwise compound assignment
				switch node.Operator {
				case "+=":
					c.emitAdd(valueReg, currentValueReg, rhsReg, line)
				case "-=":
					c.emitSubtract(valueReg, currentValueReg, rhsReg, line)
				case "*=":
					c.emitMultiply(valueReg, currentValueReg, rhsReg, line)
				case "/=":
					c.emitDivide(valueReg, currentValueReg, rhsReg, line)
				case "%=":
					c.emitRemainder(valueReg, currentValueReg, rhsReg, line)
				case "**=":
					c.emitExponent(valueReg, currentValueReg, rhsReg, line)
				case "&=":
					c.emitBitwiseAnd(valueReg, currentValueReg, rhsReg, line)
				case "|=":
					c.emitBitwiseOr(valueReg, currentValueReg, rhsReg, line)
				case "^=":
					c.emitBitwiseXor(valueReg, currentValueReg, rhsReg, line)
				case "<<=":
					c.emitShiftLeft(valueReg, currentValueReg, rhsReg, line)
				case ">>=":
					c.emitShiftRight(valueReg, currentValueReg, rhsReg, line)
				case ">>>=":
					c.emitUnsignedShiftRight(valueReg, currentValueReg, rhsReg, line)
				default:
					return BadRegister, NewCompileError(node, fmt.Sprintf("unsupported compound operator for super: %s", node.Operator))
				}
			} else {
				// Simple assignment: just use RHS value
				c.emitMove(valueReg, rhsReg, line)
			}

			// Emit OpSetSuperComputedWithBase with the captured base
			c.chunk.WriteOpCode(vm.OpSetSuperComputedWithBase, line)
			c.chunk.EmitByte(byte(baseReg))
			c.chunk.EmitByte(byte(keyReg))
			c.chunk.EmitByte(byte(valueReg))

			// Result of assignment is the assigned value
			if valueReg != hint {
				c.emitMove(hint, valueReg, line)
			}
			return hint, nil
		}

		lhsType = lhsIsIndexExpr
		// Compile array expression
		arrayReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, arrayReg)
		_, err := c.compileNode(lhsNode.Left, arrayReg)
		if err != nil {
			return BadRegister, err
		}
		indexInfo.arrayReg = arrayReg

		// Compile index expression
		indexReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, indexReg)
		_, err = c.compileNode(lhsNode.Index, indexReg)
		if err != nil {
			return BadRegister, err
		}
		indexInfo.indexReg = indexReg

		// Only load the current value for compound/logical assignment (not simple =)
		// For simple assignment, we don't need the current value and must not call
		// OpGetIndex before evaluating the RHS (per ECMAScript evaluation order)
		if node.Operator != "=" {
			currentValueReg = c.regAlloc.Alloc()
			tempRegs = append(tempRegs, currentValueReg)
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(currentValueReg))
			c.emitByte(byte(indexInfo.arrayReg))
			c.emitByte(byte(indexInfo.indexReg))
		}

	case *parser.MemberExpression:
		// Check for super property assignment (super.prop = value or super[expr] = value)
		if _, isSuper := lhsNode.Object.(*parser.SuperExpression); isSuper {
			// Super property assignment requires special handling with dedicated opcodes
			// This is because of dual-object semantics: property lookup on super base,
			// but receiver binding uses original 'this' for setters

			// Check if this is a compound assignment (not simple =)
			isCompound := node.Operator != "="

			// For computed properties: super[expr] = value or super[expr] op= value
			if computedKey, ok := lhsNode.Property.(*parser.ComputedPropertyName); ok {
				// Step 1: Capture super base FIRST (before key evaluation)
				// per ECMAScript spec (GetSuperBase before ToPropertyKey)
				baseReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, baseReg)
				c.chunk.WriteOpCode(vm.OpLoadSuper, line)
				c.chunk.EmitByte(byte(baseReg))

				// Step 2: Compile the key expression (may have side effects like changing prototype)
				keyReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, keyReg)
				_, err := c.compileNode(computedKey.Expr, keyReg)
				if err != nil {
					return BadRegister, err
				}

				var currentValueReg Register
				if isCompound {
					// Step 3: For compound assignment, read current value
					// Note: We use OpGetSuperComputed which will re-get the super base,
					// but since the key is already evaluated, this is correct for most cases
					currentValueReg = c.regAlloc.Alloc()
					tempRegs = append(tempRegs, currentValueReg)
					c.chunk.WriteOpCode(vm.OpGetSuperComputed, line)
					c.chunk.EmitByte(byte(currentValueReg)) // destination
					c.chunk.EmitByte(byte(keyReg))          // key
				}

				// Step 4: Compile the RHS value
				rhsReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, rhsReg)
				_, err = c.compileNode(node.Value, rhsReg)
				if err != nil {
					return BadRegister, err
				}

				// Step 5: Compute final value
				valueReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, valueReg)

				if isCompound {
					// Arithmetic/bitwise compound assignment
					switch node.Operator {
					case "+=":
						c.emitAdd(valueReg, currentValueReg, rhsReg, line)
					case "-=":
						c.emitSubtract(valueReg, currentValueReg, rhsReg, line)
					case "*=":
						c.emitMultiply(valueReg, currentValueReg, rhsReg, line)
					case "/=":
						c.emitDivide(valueReg, currentValueReg, rhsReg, line)
					case "%=":
						c.emitRemainder(valueReg, currentValueReg, rhsReg, line)
					case "**=":
						c.emitExponent(valueReg, currentValueReg, rhsReg, line)
					case "&=":
						c.emitBitwiseAnd(valueReg, currentValueReg, rhsReg, line)
					case "|=":
						c.emitBitwiseOr(valueReg, currentValueReg, rhsReg, line)
					case "^=":
						c.emitBitwiseXor(valueReg, currentValueReg, rhsReg, line)
					case "<<=":
						c.emitShiftLeft(valueReg, currentValueReg, rhsReg, line)
					case ">>=":
						c.emitShiftRight(valueReg, currentValueReg, rhsReg, line)
					case ">>>=":
						c.emitUnsignedShiftRight(valueReg, currentValueReg, rhsReg, line)
					default:
						return BadRegister, NewCompileError(node, fmt.Sprintf("unsupported compound operator for super: %s", node.Operator))
					}
				} else {
					// Simple assignment: just use RHS value
					c.emitMove(valueReg, rhsReg, line)
				}

				// Step 6: Write back using captured super base
				c.chunk.WriteOpCode(vm.OpSetSuperComputedWithBase, line)
				c.chunk.EmitByte(byte(baseReg))   // super base
				c.chunk.EmitByte(byte(keyReg))   // key
				c.chunk.EmitByte(byte(valueReg)) // value

				// Result of assignment is the assigned value
				if valueReg != hint {
					c.emitMove(hint, valueReg, line)
				}
				return hint, nil
			} else {
				// Static property: super.prop = value or super.prop op= value
				propName := c.extractPropertyName(lhsNode.Property)
				nameConstIdx := c.chunk.AddConstant(vm.String(propName))

				var currentValueReg Register
				if isCompound {
					// For compound assignment, read current value first
					currentValueReg = c.regAlloc.Alloc()
					tempRegs = append(tempRegs, currentValueReg)
					c.chunk.WriteOpCode(vm.OpGetSuper, line)
					c.chunk.EmitByte(byte(currentValueReg))
					c.chunk.WriteUint16(nameConstIdx)
				}

				// Compile the RHS value
				rhsReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, rhsReg)
				_, err := c.compileNode(node.Value, rhsReg)
				if err != nil {
					return BadRegister, err
				}

				// Compute final value
				valueReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, valueReg)

				if isCompound {
					// Arithmetic/bitwise compound assignment
					switch node.Operator {
					case "+=":
						c.emitAdd(valueReg, currentValueReg, rhsReg, line)
					case "-=":
						c.emitSubtract(valueReg, currentValueReg, rhsReg, line)
					case "*=":
						c.emitMultiply(valueReg, currentValueReg, rhsReg, line)
					case "/=":
						c.emitDivide(valueReg, currentValueReg, rhsReg, line)
					case "%=":
						c.emitRemainder(valueReg, currentValueReg, rhsReg, line)
					case "**=":
						c.emitExponent(valueReg, currentValueReg, rhsReg, line)
					case "&=":
						c.emitBitwiseAnd(valueReg, currentValueReg, rhsReg, line)
					case "|=":
						c.emitBitwiseOr(valueReg, currentValueReg, rhsReg, line)
					case "^=":
						c.emitBitwiseXor(valueReg, currentValueReg, rhsReg, line)
					case "<<=":
						c.emitShiftLeft(valueReg, currentValueReg, rhsReg, line)
					case ">>=":
						c.emitShiftRight(valueReg, currentValueReg, rhsReg, line)
					case ">>>=":
						c.emitUnsignedShiftRight(valueReg, currentValueReg, rhsReg, line)
					default:
						return BadRegister, NewCompileError(node, fmt.Sprintf("unsupported compound operator for super: %s", node.Operator))
					}
				} else {
					// Simple assignment: just use RHS value
					c.emitMove(valueReg, rhsReg, line)
				}

				// Emit OpSetSuper
				c.chunk.WriteOpCode(vm.OpSetSuper, line)
				c.chunk.WriteUint16(nameConstIdx)
				c.chunk.EmitByte(byte(valueReg))

				// Result of assignment is the assigned value
				if valueReg != hint {
					c.emitMove(hint, valueReg, line)
				}
				return hint, nil
			}
		}

		lhsType = lhsIsMemberExpr
		// Compile the object expression
		objectReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, objectReg)
		_, err := c.compileNode(lhsNode.Object, objectReg)
		if err != nil {
			return BadRegister, err
		}
		memberInfo.objectReg = objectReg

		// Check if this is a computed property
		if computedKey, ok := lhsNode.Property.(*parser.ComputedPropertyName); ok {
			// This is a computed property: obj[expr] = value
			memberInfo.isComputed = true
			memberInfo.keyReg = c.regAlloc.Alloc()
			tempRegs = append(tempRegs, memberInfo.keyReg)
			_, err := c.compileNode(computedKey.Expr, memberInfo.keyReg)
			if err != nil {
				return BadRegister, err
			}
		} else {
			// Regular property access: obj.prop = value
			memberInfo.isComputed = false
			propName := c.extractPropertyName(lhsNode.Property)

			// Check for private field (starts with #)
			if len(propName) > 0 && propName[0] == '#' {
				// Private field - store the field name without # prefix
				fieldName := propName[1:]
				// Use branded key to distinguish private fields with same name in different classes
				brandedKey := c.getPrivateFieldKey(fieldName)
				memberInfo.nameConstIdx = c.chunk.AddConstant(vm.String(brandedKey))
				memberInfo.isPrivateField = true

				// Check if this member is declared as a setter
				if kind, _, ok := c.getPrivateMemberKind(fieldName); ok {
					if kind == PrivateMemberSetter || kind == PrivateMemberAccessor {
						memberInfo.isPrivateSetter = true
					}
				}
			} else {
				// Regular property
				memberInfo.nameConstIdx = c.chunk.AddConstant(vm.String(propName))
				memberInfo.isPrivateField = false
			}
		}

		// If compound or logical assignment, load the current property value
		if node.Operator != "=" {
			if memberInfo.isComputed {
				// For computed properties, always use OpGetIndex since we can't know property name at compile time
				currentValueReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, currentValueReg)
				c.emitOpCode(vm.OpGetIndex, line)
				c.emitByte(byte(currentValueReg))      // Destination register
				c.emitByte(byte(memberInfo.objectReg)) // Object register
				c.emitByte(byte(memberInfo.keyReg))    // Key register (computed at runtime)
			} else if memberInfo.isPrivateField {
				// For private fields, use OpGetPrivateField
				currentValueReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, currentValueReg)
				c.emitGetPrivateField(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)
			} else {
				// For static properties, use OpGetProp which handles getters automatically
				currentValueReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, currentValueReg)
				c.emitGetProp(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)
			}
		} else {
			// For simple assignment '=', we don't need the current value
			currentValueReg = nilRegister
		}

	default:
		return BadRegister, NewCompileError(node, fmt.Sprintf("invalid assignment target, expected identifier, index expression, or member expression, got %T", node.Left))
	}

	// Track temporary registers used for operation results
	var operationTempRegs []Register

	var jumpPastStore int = -1
	var jumpToEnd int = -1

	// --- Logical Assignment Operators (&&=, ||=, ??=) ---
	if node.Operator == "&&=" || node.Operator == "||=" || node.Operator == "??=" {
		var rhsValueReg Register

		var jumpToEvalRhs int = -1
		var jumpToShortCircuit int = -1

		switch node.Operator {
		case "&&=":
			// If FALSEY -> jumpToShortCircuit (skip RHS eval AND store)
			jumpToShortCircuit = c.emitPlaceholderJump(vm.OpJumpIfFalse, currentValueReg, line)
		case "||=":
			// If FALSEY -> jumpToEvalRhs
			jumpToEvalRhs = c.emitPlaceholderJump(vm.OpJumpIfFalse, currentValueReg, line)
			// If TRUTHY -> jumpToShortCircuit (skip RHS eval AND store)
			jumpToShortCircuit = c.emitPlaceholderJump(vm.OpJump, 0, line)
		case "??=":
			// Use efficient nullish check opcode
			isNullishReg := c.regAlloc.Alloc()
			operationTempRegs = append(operationTempRegs, isNullishReg)
			c.emitIsNullish(isNullishReg, currentValueReg, line)
			// If NOT nullish -> jumpToShortCircuit (skip RHS eval AND store)
			jumpToShortCircuit = c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullishReg, line)
		}

		// --- Evaluate RHS Path ---
		if jumpToEvalRhs != -1 {
			c.patchJump(jumpToEvalRhs)
		}
		// This block is reached if short-circuit didn't happen
		rhsValueReg = c.regAlloc.Alloc()
		operationTempRegs = append(operationTempRegs, rhsValueReg)

		// Function name inference for logical assignment to identifier:
		// anonymous functions should inherit the variable name per ECMAScript spec
		var err errors.PaseratiError
		if lhsType == lhsIsIdentifier {
			if ident, ok := node.Left.(*parser.Identifier); ok {
				nameHint := ident.Value
				// Check if RHS is an anonymous function that should inherit the name
				if arrowFunc, ok := node.Value.(*parser.ArrowFunctionLiteral); ok {
					// Arrow function - compile with name hint
					funcConstIndex, freeSymbols, compileErr := c.compileArrowFunctionWithName(arrowFunc, nameHint)
					if compileErr != nil {
						return BadRegister, compileErr
					}
					// Emit closure into rhsValueReg
					var body *parser.BlockStatement
					if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
						body = blockBody
					} else {
						body = &parser.BlockStatement{}
					}
					minimalFuncLit := &parser.FunctionLiteral{Body: body}
					c.emitClosure(rhsValueReg, funcConstIndex, minimalFuncLit, freeSymbols)
				} else if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
					// Anonymous function literal - set name before compilation
					if funcLit.Name == nil || funcLit.Name.Value == "" {
						funcLit.Name = &parser.Identifier{Token: funcLit.Token, Value: nameHint}
					}
					_, err = c.compileNode(node.Value, rhsValueReg)
				} else if classExpr, ok := node.Value.(*parser.ClassExpression); ok {
					// Anonymous class expression - set name before compilation
					if classExpr.Name == nil {
						classExpr.Name = &parser.Identifier{Token: classExpr.Token, Value: nameHint}
					}
					_, err = c.compileNode(node.Value, rhsValueReg)
				} else {
					// Other RHS - compile normally
					_, err = c.compileNode(node.Value, rhsValueReg)
				}
			} else {
				_, err = c.compileNode(node.Value, rhsValueReg)
			}
		} else {
			_, err = c.compileNode(node.Value, rhsValueReg)
		}
		if err != nil {
			return BadRegister, err
		}
		debugPrintf("// DEBUG Assign Logical RHS: Evaluated RHS. rhsValueReg=R%d\n", rhsValueReg)

		// Move RHS result to hint register for final result
		if rhsValueReg != hint {
			c.emitMove(hint, rhsValueReg, line)
		}

		// Store hint to LHS (inline the store logic for RHS path)
		switch lhsType {
		case lhsIsIdentifier:
			if identInfo.isWithOrLocal {
				// Use pre-captured binding to ensure correct semantics
				c.emitSetWithByBinding(int(identInfo.withNameConstIdx), hint, identInfo.withLocalReg, identInfo.withBindingReg, line)
			} else if identInfo.isGlobal {
				c.emitSetGlobal(identInfo.globalIdx, hint, line)
			} else if identInfo.isCallerLocal {
				c.emitOpCode(vm.OpSetCallerLocal, line)
				c.emitByte(byte(identInfo.callerRegIdx))
				c.emitByte(byte(hint))
			} else if identInfo.isUpvalue {
				c.emitSetUpvalue(identInfo.upvalueIndex, hint, line)
			} else if identInfo.isSpilled {
				c.emitStoreSpill(identInfo.spillIndex, hint, line)
			} else {
				if hint != identInfo.targetReg {
					c.emitMove(identInfo.targetReg, hint, line)
				}
			}
		case lhsIsIndexExpr:
			c.emitOpCode(vm.OpSetIndex, line)
			c.emitByte(byte(indexInfo.arrayReg))
			c.emitByte(byte(indexInfo.indexReg))
			c.emitByte(byte(hint))
		case lhsIsMemberExpr:
			if memberInfo.isComputed {
				c.emitOpCode(vm.OpSetIndex, line)
				c.emitByte(byte(memberInfo.objectReg))
				c.emitByte(byte(memberInfo.keyReg))
				c.emitByte(byte(hint))
			} else if memberInfo.isPrivateField {
				if memberInfo.isPrivateSetter {
					c.emitCallPrivateSetter(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
				} else {
					c.emitSetPrivateField(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
				}
			} else {
				c.emitSetProp(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
			}
		}

		// Jump past short-circuit path
		jumpToEnd = c.emitPlaceholderJump(vm.OpJump, 0, line)

		// --- Short-circuit path (currentValue is already the result) ---
		c.patchJump(jumpToShortCircuit)
		// Move current value to hint register for final result
		if currentValueReg != hint {
			c.emitMove(hint, currentValueReg, line)
		}
		debugPrintf("// DEBUG Assign Logical ShortCircuit: skipped store\n")
		// Short-circuit path doesn't store, just returns current value in hint
		// needsStore will be set to false below to skip the store logic

		needsStore = false // Skip the store logic below (we already handled both paths)

	} else { // --- Non-Logical Assignment ---
		// Compile RHS with potential function name inference for simple assignments
		rhsValueReg := c.regAlloc.Alloc()
		operationTempRegs = append(operationTempRegs, rhsValueReg)

		// Function name inference: for simple "=" assignment to identifier,
		// anonymous functions should inherit the variable name per ECMAScript spec
		var err errors.PaseratiError
		if node.Operator == "=" && lhsType == lhsIsIdentifier {
			if ident, ok := node.Left.(*parser.Identifier); ok {
				nameHint := ident.Value
				// Check if RHS is an anonymous function that should inherit the name
				if arrowFunc, ok := node.Value.(*parser.ArrowFunctionLiteral); ok {
					// Arrow function - compile with name hint
					funcConstIndex, freeSymbols, compileErr := c.compileArrowFunctionWithName(arrowFunc, nameHint)
					if compileErr != nil {
						return BadRegister, compileErr
					}
					// Emit closure into rhsValueReg
					var body *parser.BlockStatement
					if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
						body = blockBody
					} else {
						body = &parser.BlockStatement{}
					}
					minimalFuncLit := &parser.FunctionLiteral{Body: body}
					c.emitClosure(rhsValueReg, funcConstIndex, minimalFuncLit, freeSymbols)
				} else if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
					// Anonymous function literal - set name before compilation
					if funcLit.Name == nil || funcLit.Name.Value == "" {
						funcLit.Name = &parser.Identifier{Token: funcLit.Token, Value: nameHint}
					}
					_, err = c.compileNode(node.Value, rhsValueReg)
				} else if classExpr, ok := node.Value.(*parser.ClassExpression); ok {
					// Anonymous class expression - set name before compilation
					if classExpr.Name == nil {
						classExpr.Name = &parser.Identifier{Token: classExpr.Token, Value: nameHint}
					}
					_, err = c.compileNode(node.Value, rhsValueReg)
				} else {
					// Other RHS - compile normally
					_, err = c.compileNode(node.Value, rhsValueReg)
				}
			} else {
				_, err = c.compileNode(node.Value, rhsValueReg)
			}
		} else {
			_, err = c.compileNode(node.Value, rhsValueReg)
		}
		if err != nil {
			return BadRegister, NewCompileError(node, "error compiling RHS").CausedBy(err)
		}

		// Check if we can do in-place operation on local variable
		canDoInPlace := lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg

		switch node.Operator {
		// --- Compound Arithmetic ---
		case "+=":
			if canDoInPlace {
				c.emitAdd(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false // Already stored in targetReg
			} else {
				c.emitAdd(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case "-=":
			if canDoInPlace {
				c.emitSubtract(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitSubtract(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case "*=":
			if canDoInPlace {
				c.emitMultiply(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitMultiply(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case "/=":
			if canDoInPlace {
				c.emitDivide(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitDivide(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case "%=":
			if canDoInPlace {
				c.emitRemainder(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitRemainder(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case "**=":
			if canDoInPlace {
				c.emitExponent(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitExponent(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}

		// --- Compound Bitwise / Shift ---
		case "&=":
			if canDoInPlace {
				c.emitBitwiseAnd(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitBitwiseAnd(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case "|=":
			if canDoInPlace {
				c.emitBitwiseOr(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitBitwiseOr(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case "^=":
			if canDoInPlace {
				c.emitBitwiseXor(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitBitwiseXor(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case "<<=":
			if canDoInPlace {
				c.emitShiftLeft(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitShiftLeft(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case ">>=":
			if canDoInPlace {
				c.emitShiftRight(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitShiftRight(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}
		case ">>>=":
			if canDoInPlace {
				c.emitUnsignedShiftRight(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
				needsStore = false
			} else {
				c.emitUnsignedShiftRight(hint, currentValueReg, rhsValueReg, line)
				needsStore = true
			}

		// --- Simple Assignment ---
		case "=":
			// Simple assignment: result is just the RHS value moved to hint
			if rhsValueReg != hint {
				c.emitMove(hint, rhsValueReg, line)
			}
			// Only set needsStore = true if not already set to false (e.g., for immutable bindings)
			if needsStore {
				needsStore = true // Keep as true (no-op, just for clarity)
			}
			// Note: if needsStore was set to false earlier (e.g., for NFE bindings),
			// we preserve that and skip the actual store

		default:
			return BadRegister, NewCompileError(node, fmt.Sprintf("unsupported assignment operator '%s'", node.Operator))
		}
	}

	// Clean up operation temporary registers
	for _, reg := range operationTempRegs {
		c.regAlloc.Free(reg)
	}

	// --- Store Result Back to LHS ---
	if needsStore {
		switch lhsType {
		case lhsIsIdentifier:
			if identInfo.isWithOrLocal {
				// With-property with local fallback - use pre-captured binding for correct semantics
				debugPrintf("// DEBUG Assign Store Ident: Emitting SetWithByBinding NameIdx=%d, Value=R%d, Local=R%d, Binding=R%d\n", identInfo.withNameConstIdx, hint, identInfo.withLocalReg, identInfo.withBindingReg)
				c.emitSetWithByBinding(int(identInfo.withNameConstIdx), hint, identInfo.withLocalReg, identInfo.withBindingReg, line)
			} else if identInfo.isGlobal {
				// Global variable assignment
				c.emitSetGlobal(identInfo.globalIdx, hint, line)
			} else if identInfo.isCallerLocal {
				// Caller local assignment (for direct eval)
				c.emitOpCode(vm.OpSetCallerLocal, line)
				c.emitByte(byte(identInfo.callerRegIdx))
				c.emitByte(byte(hint))
			} else if identInfo.isUpvalue {
				c.emitSetUpvalue(identInfo.upvalueIndex, hint, line)
			} else if identInfo.isSpilled {
				// Spilled variable assignment
				debugPrintf("// DEBUG Assign Store Ident: Emitting StoreSpill[%d] <- R%d\n", identInfo.spillIndex, hint)
				c.emitStoreSpill(identInfo.spillIndex, hint, line)
			} else {
				if hint != identInfo.targetReg {
					debugPrintf("// DEBUG Assign Store Ident: Emitting Move R%d <- R%d\n", identInfo.targetReg, hint)
					c.emitMove(identInfo.targetReg, hint, line)
				} else {
					debugPrintf("// DEBUG Assign Store Ident: Skipping Move R%d <- R%d (already inplace)\n", identInfo.targetReg, hint)
				}
			}
		case lhsIsIndexExpr:
			debugPrintf("// DEBUG Assign Store Index: Emitting SetIndex [%d][%d] = R%d\n", indexInfo.arrayReg, indexInfo.indexReg, hint)
			c.emitOpCode(vm.OpSetIndex, line)
			c.emitByte(byte(indexInfo.arrayReg))
			c.emitByte(byte(indexInfo.indexReg))
			c.emitByte(byte(hint))
		case lhsIsMemberExpr:
			// OpSetProp now properly handles accessor properties via OpDefineAccessor
			if memberInfo.isWithProperty {
				// Use OpSetWithProperty for with properties
				debugPrintf("// DEBUG Assign Store With Property: Emitting OpSetWithProperty[%d] = R%d\n", memberInfo.nameConstIdx, hint)
				c.emitSetWithProperty(int(memberInfo.nameConstIdx), hint, line)
			} else if memberInfo.isComputed {
				// Use OpSetIndex for computed properties: objectReg[keyReg] = hint
				debugPrintf("// DEBUG Assign Store Member: Emitting SetIndex R%d[R%d] = R%d\n", memberInfo.objectReg, memberInfo.keyReg, hint)
				c.emitOpCode(vm.OpSetIndex, line)
				c.emitByte(byte(memberInfo.objectReg)) // Object register
				c.emitByte(byte(memberInfo.keyReg))    // Key register (computed at runtime)
				c.emitByte(byte(hint))                 // Value register
			} else if memberInfo.isPrivateField {
				// Use OpSetPrivateField for private fields: objectReg.#field = hint
				debugPrintf("// DEBUG Assign Store Private Field: Emitting SetPrivateField R%d[%d] = R%d\n", memberInfo.objectReg, memberInfo.nameConstIdx, hint)
				if memberInfo.isPrivateSetter {
					c.emitCallPrivateSetter(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
				} else {
					c.emitSetPrivateField(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
				}
			} else {
				// Use OpSetProp for static properties: objectReg.nameConstIdx = hint
				// OpSetProp will invoke setters if the property is an accessor
				debugPrintf("// DEBUG Assign Store Member: Emitting SetProp R%d[%d] = R%d\n", memberInfo.objectReg, memberInfo.nameConstIdx, hint)
				c.emitSetProp(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
			}
		}
	} else {
		debugPrintf("// DEBUG Assign Store: Skipped store operation (needsStore=false)\n")
	}

	// --- Final Merge Point & Patching ---
	if jumpPastStore != -1 {
		c.patchJump(jumpPastStore)
	}
	if jumpToEnd != -1 {
		c.patchJump(jumpToEnd)
	}

	if debugAssignment {
		fmt.Printf("// DEBUG Assignment Finalize: hint=R%d\n", hint)
	}

	return hint, nil
}

// compileArrayDestructuringAssignment compiles array destructuring like [a, b, c] = expr
// Uses the iterator protocol per ECMAScript spec: gets iterator, calls next() for each element,
// and calls iterator.return() when finished early (IteratorClose)
func (c *Compiler) compileArrayDestructuringAssignment(node *parser.ArrayDestructuringAssignment, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	if debugAssignment {
		fmt.Printf("// [Assignment] Compiling array destructuring: %s\n", node.String())
	}

	// 1. Compile RHS expression into iterable register
	iterableReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iterableReg)

	_, err := c.compileNode(node.Value, iterableReg)
	if err != nil {
		return BadRegister, err
	}

	// Save the original value for returning (assignment expression returns the RHS value)
	if hint != iterableReg {
		c.emitMove(hint, iterableReg, line)
	}

	// 2. Get Symbol.iterator method
	iteratorMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorMethodReg)

	// Load global Symbol
	symbolObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(symbolObjReg)
	symIdx := c.GetOrAssignGlobalIndex("Symbol")
	c.emitGetGlobal(symbolObjReg, symIdx, line)

	// Get Symbol.iterator
	propNameReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(propNameReg)
	c.emitLoadNewConstant(propNameReg, vm.String("iterator"), line)

	iteratorKeyReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorKeyReg)
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorKeyReg))
	c.emitByte(byte(symbolObjReg))
	c.emitByte(byte(propNameReg))

	// Get iterable[Symbol.iterator]
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorMethodReg))
	c.emitByte(byte(iterableReg))
	c.emitByte(byte(iteratorKeyReg))

	// 3. Call the iterator method to get iterator object
	iteratorObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorObjReg)
	c.emitCallMethod(iteratorObjReg, iteratorMethodReg, iterableReg, 0, line)

	// 4. Allocate register to track iterator.done state
	doneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(doneReg)
	c.emitLoadFalse(doneReg, line)

	// 5. Mark start of iterator cleanup region - any exception or generator.return()
	// during element processing should close the iterator
	iteratorCleanupTryStart := len(c.chunk.Code)

	// 6. For each element, call iterator.next() and assign
	for _, element := range node.Elements {
		if element.Target == nil {
			// Elision: consume iterator value but don't bind
			c.compileIteratorNext(iteratorObjReg, BadRegister, doneReg, line, true)
			continue
		}

		if element.IsRest {
			// Rest element: collect all remaining iterator values into an array
			// Per ECMAScript spec: evaluate lref FIRST, then collect values, then assign
			// This is important for cases like [...obj[yield]] = iter where yield
			// must happen before consuming the iterator

			var baseReg, propReg Register = BadRegister, BadRegister
			var identSymbol Symbol
			var identFound bool
			var isSimpleTarget bool

			// Check target type - only evaluate lref for non-pattern targets
			switch targetNode := element.Target.(type) {
			case *parser.Identifier:
				// Simple identifier - resolve now but don't assign yet
				identSymbol, _, identFound = c.currentSymbolTable.Resolve(targetNode.Value)
				isSimpleTarget = true

			case *parser.MemberExpression:
				// obj.prop - evaluate obj now
				baseReg = c.regAlloc.Alloc()
				_, err := c.compileNode(targetNode.Object, baseReg)
				if err != nil {
					c.regAlloc.Free(baseReg)
					return BadRegister, err
				}
				isSimpleTarget = true

			case *parser.IndexExpression:
				// obj[key] - evaluate both obj and key now
				baseReg = c.regAlloc.Alloc()
				_, err := c.compileNode(targetNode.Left, baseReg)
				if err != nil {
					c.regAlloc.Free(baseReg)
					return BadRegister, err
				}
				propReg = c.regAlloc.Alloc()
				_, err = c.compileNode(targetNode.Index, propReg)
				if err != nil {
					c.regAlloc.Free(baseReg)
					c.regAlloc.Free(propReg)
					return BadRegister, err
				}
				isSimpleTarget = true

			default:
				// Nested pattern (ArrayLiteral, ObjectLiteral) - handle normally
				isSimpleTarget = false
			}

			// Now collect values from iterator
			// Pass doneReg so it gets updated if next() throws (for proper exception handling)
			restArrayReg := c.regAlloc.Alloc()
			err := c.compileIteratorToArrayWithDone(iteratorObjReg, restArrayReg, doneReg, line)
			if err != nil {
				c.regAlloc.Free(restArrayReg)
				if baseReg != BadRegister {
					c.regAlloc.Free(baseReg)
				}
				if propReg != BadRegister {
					c.regAlloc.Free(propReg)
				}
				return BadRegister, err
			}

			// Now assign to the pre-evaluated target
			if isSimpleTarget {
				switch targetNode := element.Target.(type) {
				case *parser.Identifier:
					if identFound {
						// Check for const assignment
						if identSymbol.IsConst {
							c.emitConstAssignmentError(targetNode.Value, line)
						} else if identSymbol.IsGlobal {
							c.emitSetGlobal(identSymbol.GlobalIndex, restArrayReg, line)
						} else if identSymbol.IsSpilled {
							c.emitStoreSpill(identSymbol.SpillIndex, restArrayReg, line)
						} else {
							if restArrayReg != identSymbol.Register {
								c.emitMove(identSymbol.Register, restArrayReg, line)
							}
						}
					} else if c.chunk.IsStrict {
						c.emitStrictUnresolvableReferenceError(targetNode.Value, line)
					} else {
						// Implicit global in non-strict
						globalIdx := c.GetOrAssignGlobalIndex(targetNode.Value)
						c.emitSetGlobal(globalIdx, restArrayReg, line)
					}

				case *parser.MemberExpression:
					// Use pre-evaluated base with property name
					propName, ok := targetNode.Property.(*parser.Identifier)
					if !ok {
						c.regAlloc.Free(baseReg)
						c.regAlloc.Free(restArrayReg)
						return BadRegister, NewCompileError(targetNode, "member expression property must be an identifier")
					}
					propConstIdx := c.chunk.AddConstant(vm.String(propName.Value))
					c.emitSetProp(baseReg, restArrayReg, propConstIdx, line)
					c.regAlloc.Free(baseReg)

				case *parser.IndexExpression:
					// Use pre-evaluated base and property
					c.emitOpCode(vm.OpSetIndex, line)
					c.emitByte(byte(baseReg))
					c.emitByte(byte(propReg))
					c.emitByte(byte(restArrayReg))
					c.regAlloc.Free(baseReg)
					c.regAlloc.Free(propReg)
				}
			} else {
				// Nested pattern - handle normally
				if element.Default != nil {
					err = c.compileConditionalAssignment(element.Target, restArrayReg, element.Default, line)
				} else {
					err = c.compileSimpleAssignment(element.Target, restArrayReg, line)
				}
			}

			c.regAlloc.Free(restArrayReg)
			if err != nil {
				return BadRegister, err
			}

			// Rest must be last, the iterator is exhausted
			c.emitLoadTrue(doneReg, line)
			break
		}

		// Regular element: per ECMAScript spec, evaluate lref FIRST, then call next()
		// This matters for cases like [ {}[thrower()] ] = iterable where thrower() should
		// execute BEFORE next() is called
		var elemBaseReg, elemPropReg Register = BadRegister, BadRegister
		var elemIsIndexExpr bool

		// Check target type - only pre-evaluate lref for IndexExpression
		// (MemberExpression is always non-computed so no need for special handling there,
		// and Identifier just resolves a name with no side effects)
		switch targetNode := element.Target.(type) {
		case *parser.IndexExpression:
			// obj[key] - evaluate both obj and key now BEFORE calling next()
			elemBaseReg = c.regAlloc.Alloc()
			_, err := c.compileNode(targetNode.Left, elemBaseReg)
			if err != nil {
				c.regAlloc.Free(elemBaseReg)
				return BadRegister, err
			}
			elemPropReg = c.regAlloc.Alloc()
			_, err = c.compileNode(targetNode.Index, elemPropReg)
			if err != nil {
				c.regAlloc.Free(elemBaseReg)
				c.regAlloc.Free(elemPropReg)
				return BadRegister, err
			}
			elemIsIndexExpr = true
		}

		// NOW get next value from iterator
		valueReg := c.regAlloc.Alloc()
		c.compileIteratorNext(iteratorObjReg, valueReg, doneReg, line, false)

		// Handle assignment
		if elemIsIndexExpr {
			// IndexExpression with pre-evaluated lref
			// Handle default value first if present
			if element.Default != nil {
				// Conditional: if value is undefined, use default
				jumpToDefault := c.emitPlaceholderJump(vm.OpJumpIfUndefined, valueReg, line)

				// Value is not undefined - assign to pre-evaluated target
				c.emitOpCode(vm.OpSetIndex, line)
				c.emitByte(byte(elemBaseReg))
				c.emitByte(byte(elemPropReg))
				c.emitByte(byte(valueReg))
				jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)

				// Value is undefined - evaluate and assign default
				c.patchJump(jumpToDefault)
				defaultReg := c.regAlloc.Alloc()
				_, err = c.compileNode(element.Default, defaultReg)
				if err != nil {
					c.regAlloc.Free(defaultReg)
					c.regAlloc.Free(elemBaseReg)
					c.regAlloc.Free(elemPropReg)
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
				c.emitOpCode(vm.OpSetIndex, line)
				c.emitByte(byte(elemBaseReg))
				c.emitByte(byte(elemPropReg))
				c.emitByte(byte(defaultReg))
				c.regAlloc.Free(defaultReg)
				c.patchJump(jumpPastDefault)
			} else {
				// No default - just assign
				c.emitOpCode(vm.OpSetIndex, line)
				c.emitByte(byte(elemBaseReg))
				c.emitByte(byte(elemPropReg))
				c.emitByte(byte(valueReg))
			}
			c.regAlloc.Free(elemBaseReg)
			c.regAlloc.Free(elemPropReg)
		} else {
			// Use existing assignment logic for Identifier, MemberExpression, and patterns
			if element.Default != nil {
				err = c.compileConditionalAssignment(element.Target, valueReg, element.Default, line)
			} else {
				err = c.compileSimpleAssignment(element.Target, valueReg, line)
			}
		}
		c.regAlloc.Free(valueReg)
		if err != nil {
			return BadRegister, err
		}
	}

	// 7. Call IteratorClose (iterator.return if it exists AND iterator is not done)
	c.emitIteratorCleanupWithDone(iteratorObjReg, doneReg, line)

	// 8. Mark end of iterator cleanup region - AFTER the normal cleanup
	// This ensures that if a yield happens mid-destructuring, the resume PC is still covered
	iteratorCleanupTryEnd := len(c.chunk.Code)

	// 9. Add exception handler for iterator cleanup when exception/generator.return() propagates out
	// Per ECMAScript spec, when an abrupt completion occurs during destructuring,
	// iterator.return() must be called with error suppression.
	// However, if the exception came from iterator.next() itself, we should NOT
	// call return() (per spec, the iterator is considered "done" when next() throws).
	// Jump over the handler code in normal flow
	skipHandlerJump := c.emitPlaceholderJump(vm.OpJump, 0, line)

	// Exception handler code: check done flag, call iterator.return() if not done, then re-throw
	iteratorCleanupHandlerPC := len(c.chunk.Code)
	c.emitIteratorCleanupAbruptIfNotDone(iteratorObjReg, doneReg, line)
	c.emitHandlePendingAction(line)

	// Patch the skip jump to land after the handler
	c.patchJump(skipHandlerJump)

	// Add the exception handler to the exception table
	// This is an iterator cleanup handler - triggered by exceptions AND generator.return()
	iteratorCleanupHandler := vm.ExceptionHandler{
		TryStart:          iteratorCleanupTryStart,
		TryEnd:            iteratorCleanupTryEnd,
		HandlerPC:         iteratorCleanupHandlerPC,
		CatchReg:          -1,
		IsCatch:           false,
		IsFinally:         false,
		IsIteratorCleanup: true,
		FinallyReg:        -1,
	}
	c.chunk.ExceptionTable = append(c.chunk.ExceptionTable, iteratorCleanupHandler)

	return hint, nil
}

// compileArraySliceCall compiles an array slice operation for rest elements
func (c *Compiler) compileArraySliceCall(arrayReg Register, startIndex int, resultReg Register, line int) errors.PaseratiError {
	// This compiles: resultReg = arrayReg.slice(startIndex) using the specialized OpArraySlice opcode

	// Load the start index as a constant
	startIndexReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(startIndexReg)

	startConstIdx := c.chunk.AddConstant(vm.Number(float64(startIndex)))
	c.emitLoadConstant(startIndexReg, startConstIdx, line)

	// Emit the array slice opcode: OpArraySlice destReg arrayReg startIndexReg
	c.emitOpCode(vm.OpArraySlice, line)
	c.emitByte(byte(resultReg))     // destination register
	c.emitByte(byte(arrayReg))      // source array register
	c.emitByte(byte(startIndexReg)) // start index register

	return nil
}

// compileSimpleAssignment handles assignment to a single target (supports nested patterns)
func (c *Compiler) compileSimpleAssignment(target parser.Expression, valueReg Register, line int) errors.PaseratiError {
	return c.compileRecursiveAssignment(target, valueReg, line)
}

// compileRecursiveAssignment handles assignment to various target types including nested patterns
func (c *Compiler) compileRecursiveAssignment(target parser.Expression, valueReg Register, line int) errors.PaseratiError {
	switch targetNode := target.(type) {
	case *parser.Identifier:
		// Simple variable assignment: a = value
		return c.compileIdentifierAssignment(targetNode, valueReg, line)

	case *parser.MemberExpression:
		// Member expression assignment: obj.prop = value
		return c.compileMemberExpressionAssignment(targetNode, valueReg, line)

	case *parser.IndexExpression:
		// Index expression assignment: arr[0] = value
		return c.compileIndexExpressionAssignment(targetNode, valueReg, line)

	case *parser.ArrayLiteral:
		// Nested array destructuring: [a, b] = value
		return c.compileNestedArrayDestructuring(targetNode, valueReg, line)

	case *parser.ObjectLiteral:
		// Nested object destructuring: {a, b} = value
		return c.compileNestedObjectDestructuring(targetNode, valueReg, line)

	case *parser.UndefinedLiteral:
		// Elision/hole in destructuring pattern: [,] - skip assignment
		return nil

	default:
		return NewCompileError(target, fmt.Sprintf("unsupported destructuring target type: %T", target))
	}
}

// compileIdentifierAssignment handles simple variable assignment
func (c *Compiler) compileIdentifierAssignment(identTarget *parser.Identifier, valueReg Register, line int) errors.PaseratiError {
	// First check if this is from a with object (flagged by type checker)
	if identTarget.IsFromWith {
		if objReg, withFound := c.currentSymbolTable.ResolveWithProperty(identTarget.Value); withFound {
			// Emit property assignment bytecode: objReg[identTarget.Value] = valueReg
			propName := c.chunk.AddConstant(vm.String(identTarget.Value))
			c.emitSetProp(objReg, valueReg, propName, line)
			return nil
		}
	}

	// Resolve the identifier to determine how to store it
	symbol, _, found := c.currentSymbolTable.Resolve(identTarget.Value)
	if !found {
		// Variable not found in any scope
		// In strict mode, this is a ReferenceError per ECMAScript spec
		if c.chunk.IsStrict {
			c.emitStrictUnresolvableReferenceError(identTarget.Value, line)
			return nil
		}
		// In non-strict mode, treat as implicit global assignment
		globalIdx := c.GetOrAssignGlobalIndex(identTarget.Value)
		c.emitSetGlobal(globalIdx, valueReg, line)
		return nil
	}

	// Check for const assignment - emit TypeError at runtime
	if symbol.IsConst {
		c.emitConstAssignmentError(identTarget.Value, line)
		return nil
	}

	// Generate appropriate store instruction based on symbol type
	if symbol.IsGlobal {
		c.emitSetGlobal(symbol.GlobalIndex, valueReg, line)
	} else if symbol.IsSpilled {
		// For spilled variables, store to spill slot
		c.emitStoreSpill(symbol.SpillIndex, valueReg, line)
	} else {
		// For local variables, move to the allocated register
		if valueReg != symbol.Register {
			c.emitMove(symbol.Register, valueReg, line)
		}
	}

	return nil
}

// compileMemberExpressionAssignment handles assignment to object properties: obj.prop = value
func (c *Compiler) compileMemberExpressionAssignment(memberExpr *parser.MemberExpression, valueReg Register, line int) errors.PaseratiError {
	// Compile the object expression
	objReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(objReg)
	_, err := c.compileNode(memberExpr.Object, objReg)
	if err != nil {
		return err
	}

	// MemberExpression is always non-computed (obj.prop), not obj[prop]
	// For computed access, the parser creates an IndexExpression
	propName, ok := memberExpr.Property.(*parser.Identifier)
	if !ok {
		return NewCompileError(memberExpr, "member expression property must be an identifier")
	}
	propIdx := c.chunk.AddConstant(vm.String(propName.Value))
	c.emitSetProp(objReg, valueReg, propIdx, line)

	return nil
}

// compileIndexExpressionAssignment handles assignment to indexed elements: arr[0] = value
func (c *Compiler) compileIndexExpressionAssignment(indexExpr *parser.IndexExpression, valueReg Register, line int) errors.PaseratiError {
	// Compile the left side (array/object)
	leftReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(leftReg)
	_, err := c.compileNode(indexExpr.Left, leftReg)
	if err != nil {
		return err
	}

	// Compile the index expression
	indexReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(indexReg)
	_, err = c.compileNode(indexExpr.Index, indexReg)
	if err != nil {
		return err
	}

	// Emit OpSetIndex: left[index] = value
	c.emitOpCode(vm.OpSetIndex, line)
	c.emitByte(byte(leftReg))
	c.emitByte(byte(indexReg))
	c.emitByte(byte(valueReg))

	return nil
}

// compileNestedArrayDestructuring compiles nested array patterns like [a, [b, c]] = value
func (c *Compiler) compileNestedArrayDestructuring(arrayLit *parser.ArrayLiteral, valueReg Register, line int) errors.PaseratiError {
	// Convert ArrayLiteral to ArrayDestructuringAssignment for reuse of existing logic
	destructureAssign := &parser.ArrayDestructuringAssignment{
		Token: arrayLit.Token,
		Value: nil, // Will not be used since we already have the valueReg
	}

	// Convert array elements to destructuring elements
	for i, element := range arrayLit.Elements {
		var target parser.Expression
		var defaultValue parser.Expression
		var isRest bool

		// Check if this element is a rest element (...rest)
		if spreadExpr, ok := element.(*parser.SpreadElement); ok {
			// This is a rest element: [...rest]
			target = spreadExpr.Argument
			defaultValue = nil
			isRest = true
		} else if assignExpr, ok := element.(*parser.AssignmentExpression); ok && assignExpr.Operator == "=" {
			// This is a default value: [a = 5]
			target = assignExpr.Left
			defaultValue = assignExpr.Value
			isRest = false
		} else {
			// This is a simple element: [a] or nested pattern: [[a, b]]
			target = element
			defaultValue = nil
			isRest = false
		}

		destElement := &parser.DestructuringElement{
			Target:  target,
			Default: defaultValue,
			IsRest:  isRest,
		}

		// Validate rest element placement
		if isRest && i != len(arrayLit.Elements)-1 {
			return NewCompileError(arrayLit, "rest element must be last element in destructuring pattern")
		}

		destructureAssign.Elements = append(destructureAssign.Elements, destElement)
	}

	// Reuse existing compilation logic but with direct value register
	return c.compileArrayDestructuringWithValueReg(destructureAssign, valueReg, line)
}

// compileNestedObjectDestructuring compiles nested object patterns like {user: {name, age}} = value
func (c *Compiler) compileNestedObjectDestructuring(objectLit *parser.ObjectLiteral, valueReg Register, line int) errors.PaseratiError {
	// Convert ObjectLiteral to ObjectDestructuringAssignment for reuse of existing logic
	destructureAssign := &parser.ObjectDestructuringAssignment{
		Token: objectLit.Token,
		Value: nil, // Will not be used since we already have the valueReg
	}

	// Convert object properties to destructuring properties
	for _, pair := range objectLit.Properties {
		// Check for spread element first - this is a rest property like {...rest}
		if spread, ok := pair.Key.(*parser.SpreadElement); ok {
			// SpreadElement's Argument is the target identifier (e.g., 'rest' in '...rest')
			if ident, ok := spread.Argument.(*parser.Identifier); ok {
				destructureAssign.RestProperty = &parser.DestructuringElement{
					Target: ident,
					IsRest: true,
				}
			}
			// Rest must be last, so we can continue and it will be processed at the end
			continue
		}

		// Extract the key - can be identifier, string literal, number literal, or computed
		var keyExpr parser.Expression
		var keyName string
		var isShorthand bool

		switch k := pair.Key.(type) {
		case *parser.Identifier:
			keyExpr = k
			keyName = k.Value
			isShorthand = true // Could be shorthand
		case *parser.StringLiteral:
			// String literal key like {"foo": x}
			keyExpr = &parser.Identifier{Value: k.Value}
			keyName = k.Value
		case *parser.NumberLiteral:
			// Numeric key like {0: x} - use the token literal as string
			keyExpr = &parser.Identifier{Value: k.Token.Literal}
			keyName = k.Token.Literal
		case *parser.ComputedPropertyName:
			// Computed key - pass through for runtime evaluation
			keyExpr = k
			keyName = ""
		default:
			return NewCompileError(objectLit, fmt.Sprintf("invalid destructuring property key: %s", pair.Key.String()))
		}

		var target parser.Expression
		var defaultValue parser.Expression

		// Check for different patterns:
		// 1. {name} - shorthand without default
		// 2. {name = defaultVal} - shorthand with default (value is assignment expr)
		// 3. {name: localVar} - explicit target without default
		// 4. {name: localVar = defaultVal} - explicit target with default
		// 5. {name: [a, b]} - nested pattern target
		// 6. {name: {x, y}} - nested pattern target

		if isShorthand {
			if valueIdent, ok := pair.Value.(*parser.Identifier); ok && valueIdent.Value == keyName {
				// Pattern 1: Shorthand without default {name}
				target = valueIdent
				defaultValue = nil
			} else if assignExpr, ok := pair.Value.(*parser.AssignmentExpression); ok && assignExpr.Operator == "=" {
				if leftIdent, ok := assignExpr.Left.(*parser.Identifier); ok && leftIdent.Value == keyName {
					// Pattern 2: Shorthand with default {name = defaultVal}
					target = leftIdent
					defaultValue = assignExpr.Value
				} else {
					// Pattern 4: Explicit target with default {name: localVar = defaultVal}
					target = assignExpr.Left
					defaultValue = assignExpr.Value
				}
			} else {
				// Pattern 3, 5, 6: Explicit target without default
				target = pair.Value
				defaultValue = nil
			}
		} else {
			// Non-shorthand (string/number/computed key) - always explicit target
			if assignExpr, ok := pair.Value.(*parser.AssignmentExpression); ok && assignExpr.Operator == "=" {
				// Pattern 4: Explicit target with default {0: x = defaultVal}
				target = assignExpr.Left
				defaultValue = assignExpr.Value
			} else {
				// Pattern 3, 5, 6: Explicit target without default {0: x}
				target = pair.Value
				defaultValue = nil
			}
		}

		destProperty := &parser.DestructuringProperty{
			Key:     keyExpr,
			Target:  target,
			Default: defaultValue,
		}

		destructureAssign.Properties = append(destructureAssign.Properties, destProperty)
	}

	// Reuse existing compilation logic but with direct value register
	return c.compileObjectDestructuringWithValueReg(destructureAssign, valueReg, line)
}

// compileArrayDestructuringWithValueReg compiles array destructuring using an existing value register
// Uses the iterator protocol per ECMAScript spec
func (c *Compiler) compileArrayDestructuringWithValueReg(node *parser.ArrayDestructuringAssignment, iterableReg Register, line int) errors.PaseratiError {
	// 1. Get Symbol.iterator method
	iteratorMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorMethodReg)

	// Load global Symbol
	symbolObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(symbolObjReg)
	symIdx := c.GetOrAssignGlobalIndex("Symbol")
	c.emitGetGlobal(symbolObjReg, symIdx, line)

	// Get Symbol.iterator
	propNameReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(propNameReg)
	c.emitLoadNewConstant(propNameReg, vm.String("iterator"), line)

	iteratorKeyReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorKeyReg)
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorKeyReg))
	c.emitByte(byte(symbolObjReg))
	c.emitByte(byte(propNameReg))

	// Get iterable[Symbol.iterator]
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorMethodReg))
	c.emitByte(byte(iterableReg))
	c.emitByte(byte(iteratorKeyReg))

	// 2. Call the iterator method to get iterator object
	iteratorObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorObjReg)
	c.emitCallMethod(iteratorObjReg, iteratorMethodReg, iterableReg, 0, line)

	// 3. Allocate register to track iterator.done state
	doneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(doneReg)
	c.emitLoadFalse(doneReg, line)

	// 4. For each element, call iterator.next() and assign
	for _, element := range node.Elements {
		if element.Target == nil {
			// Elision: consume iterator value but don't bind
			c.compileIteratorNext(iteratorObjReg, BadRegister, doneReg, line, true)
			continue
		}

		if element.IsRest {
			// Rest element: collect all remaining iterator values into an array
			restArrayReg := c.regAlloc.Alloc()
			err := c.compileIteratorToArray(iteratorObjReg, restArrayReg, line)
			if err != nil {
				c.regAlloc.Free(restArrayReg)
				return err
			}

			// Assign rest array to target
			var err2 errors.PaseratiError
			if element.Default != nil {
				err2 = c.compileConditionalAssignment(element.Target, restArrayReg, element.Default, line)
			} else {
				err2 = c.compileRecursiveAssignment(element.Target, restArrayReg, line)
			}
			c.regAlloc.Free(restArrayReg)
			if err2 != nil {
				return err2
			}

			// Rest must be last, the iterator is exhausted
			c.emitLoadTrue(doneReg, line)
			break
		}

		// Regular element: get next value from iterator
		extractedReg := c.regAlloc.Alloc()
		c.compileIteratorNext(iteratorObjReg, extractedReg, doneReg, line, false)

		// Handle assignment with potential default value (recursive assignment)
		if element.Default != nil {
			err := c.compileConditionalAssignment(element.Target, extractedReg, element.Default, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		} else {
			err := c.compileRecursiveAssignment(element.Target, extractedReg, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		}

		c.regAlloc.Free(extractedReg)
	}

	// 5. Call IteratorClose (iterator.return if it exists AND iterator is not done)
	c.emitIteratorCleanupWithDone(iteratorObjReg, doneReg, line)

	return nil
}

// compileObjectDestructuringWithValueReg compiles object destructuring using an existing value register
func (c *Compiler) compileObjectDestructuringWithValueReg(node *parser.ObjectDestructuringAssignment, valueReg Register, line int) errors.PaseratiError {
	// Per ECMAScript spec: RequireObjectCoercible check
	// Throw TypeError if value is null or undefined (even for empty patterns like {})
	c.emitDestructuringNullCheck(valueReg, line)

	// Reuse existing object destructuring logic but skip RHS compilation
	// For each property, compile: target = valueReg.propertyName
	for _, prop := range node.Properties {
		if prop.Target == nil {
			continue // Skip malformed properties
		}

		// Allocate register for extracted property value
		extractedReg := c.regAlloc.Alloc()

		// Handle property access (identifier or computed)
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			// Static property key
			propNameIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
			c.emitGetProp(extractedReg, valueReg, propNameIdx, line)
		} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
			// Computed property key - evaluate expression
			keyReg := c.regAlloc.Alloc()
			_, err := c.compileNode(computed.Expr, keyReg)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				c.regAlloc.Free(keyReg)
				return err
			}
			// Use GetIndex for dynamic property access
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(extractedReg)) // Destination
			c.emitByte(byte(valueReg))     // Object
			c.emitByte(byte(keyReg))       // Key
			c.regAlloc.Free(keyReg)
		}

		// Handle assignment with potential default value (recursive assignment)
		if prop.Default != nil {
			// Compile conditional assignment: target = extractedReg !== undefined ? extractedReg : default
			err := c.compileConditionalAssignment(prop.Target, extractedReg, prop.Default, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		} else {
			// Recursive assignment: target = extractedReg (may be nested pattern)
			err := c.compileRecursiveAssignment(prop.Target, extractedReg, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		}

		// Clean up temporary register
		c.regAlloc.Free(extractedReg)
	}

	// Handle rest property if present
	if node.RestProperty != nil {
		err := c.compileObjectRestProperty(valueReg, node.Properties, node.RestProperty, line)
		if err != nil {
			return err
		}
	}

	return nil
}

// compileObjectDestructuringAssignment compiles object destructuring like {a, b} = expr
// Desugars into: temp = expr; a = temp.a; b = temp.b;
func (c *Compiler) compileObjectDestructuringAssignment(node *parser.ObjectDestructuringAssignment, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	if debugAssignment {
		fmt.Printf("// [Assignment] Compiling object destructuring: %s\n", node.String())
	}

	// 1. Compile RHS expression into temp register
	tempReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(tempReg)

	_, err := c.compileNode(node.Value, tempReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. For each property, compile: target = temp.propertyName
	for _, prop := range node.Properties {
		if prop.Target == nil {
			continue // Skip malformed properties
		}

		// Allocate register for extracted property value
		valueReg := c.regAlloc.Alloc()

		// Handle property access (identifier or computed)
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			propNameIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
			c.emitGetProp(valueReg, tempReg, propNameIdx, line)
		} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
			keyReg := c.regAlloc.Alloc()
			_, err := c.compileNode(computed.Expr, keyReg)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(keyReg)
				return BadRegister, err
			}
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(valueReg))
			c.emitByte(byte(tempReg))
			c.emitByte(byte(keyReg))
			c.regAlloc.Free(keyReg)
		}

		// Handle assignment with potential default value
		if prop.Default != nil {
			// Compile conditional assignment: target = valueReg !== undefined ? valueReg : default
			err := c.compileConditionalAssignment(prop.Target, valueReg, prop.Default, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		} else {
			// Simple assignment: target = valueReg
			err := c.compileSimpleAssignment(prop.Target, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		}

		// Clean up temporary register
		c.regAlloc.Free(valueReg)
	}

	// 2.5. Handle rest property if present
	if node.RestProperty != nil {
		err := c.compileObjectRestProperty(tempReg, node.Properties, node.RestProperty, line)
		if err != nil {
			return BadRegister, err
		}
	}

	// 3. Return the original RHS value (like regular assignment)
	if hint != tempReg {
		c.emitMove(hint, tempReg, line)
	}

	return hint, nil
}

// compileConditionalAssignment compiles: target = (valueReg !== undefined) ? valueReg : defaultExpr
func (c *Compiler) compileConditionalAssignment(target parser.Expression, valueReg Register, defaultExpr parser.Expression, line int) errors.PaseratiError {
	// This implements: target = valueReg !== undefined ? valueReg : defaultExpr

	// 1. Conditional jump: if undefined, jump to default value assignment
	jumpToDefault := c.emitPlaceholderJump(vm.OpJumpIfUndefined, valueReg, line)

	// 3. Path 1: Value is not undefined, assign valueReg to target
	err := c.compileSimpleAssignment(target, valueReg, line)
	if err != nil {
		// CRITICAL: Must patch jump before returning on error!
		c.patchJump(jumpToDefault)
		return err
	}

	// Jump past the default assignment
	jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)

	// 4. Path 2: Value is undefined, evaluate and assign default
	c.patchJump(jumpToDefault)

	// Compile the default expression
	defaultReg := c.regAlloc.Alloc()
	// NOTE: Don't use defer here! When called in a loop (array destructuring),
	// defer would accumulate registers. We'll free it manually at the end.

	// Check if we should apply function name inference
	// Per ECMAScript spec: if target is an identifier and default is anonymous function, use target name
	var nameHint string
	if ident, ok := target.(*parser.Identifier); ok {
		nameHint = ident.Value
	}

	// Compile default with potential name hint for anonymous functions
	if nameHint != "" {
		if funcLit, ok := defaultExpr.(*parser.FunctionLiteral); ok && funcLit.Name == nil {
			// Anonymous function literal - use target name
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, nameHint)
			if err != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return err
			}
			c.emitClosure(defaultReg, funcConstIndex, funcLit, freeSymbols)
		} else if classExpr, ok := defaultExpr.(*parser.ClassExpression); ok && classExpr.Name == nil {
			// Anonymous class expression - give it the target name temporarily
			// This allows function name inference per ECMAScript spec
			classExpr.Name = &parser.Identifier{
				Token: classExpr.Token,
				Value: nameHint,
			}
			_, err = c.compileNode(classExpr, defaultReg)
			// Restore to anonymous (though it doesn't matter since we're done compiling)
			classExpr.Name = nil
			if err != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return err
			}
		} else if arrowFunc, ok := defaultExpr.(*parser.ArrowFunctionLiteral); ok {
			// Arrow function - compile with name hint by using compileArrowFunctionWithName
			funcConstIndex, freeSymbols, err := c.compileArrowFunctionWithName(arrowFunc, nameHint)
			if err != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return err
			}
			// Create a minimal FunctionLiteral for emitClosure
			// Arrow function body can be an expression or block, wrap it appropriately
			var body *parser.BlockStatement
			if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
				body = blockBody
			} else {
				// Expression body - wrap in block for emitClosure
				body = &parser.BlockStatement{}
			}
			minimalFuncLit := &parser.FunctionLiteral{Body: body}
			c.emitClosure(defaultReg, funcConstIndex, minimalFuncLit, freeSymbols)
		} else {
			// Not a function, compile normally
			_, err = c.compileNode(defaultExpr, defaultReg)
			if err != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return err
			}
		}
	} else {
		_, err = c.compileNode(defaultExpr, defaultReg)
		if err != nil {
			c.patchJump(jumpPastDefault)
			c.regAlloc.Free(defaultReg)
			return err
		}
	}

	// Assign default value to target
	err = c.compileSimpleAssignment(target, defaultReg, line)
	if err != nil {
		c.patchJump(jumpPastDefault)
		c.regAlloc.Free(defaultReg)
		return err
	}

	// 5. Patch the jump past default
	c.patchJump(jumpPastDefault)

	// Free the default register now that we're done with it
	c.regAlloc.Free(defaultReg)

	return nil
}

// compileObjectRestProperty compiles rest property assignment for object destructuring
func (c *Compiler) compileObjectRestProperty(objReg Register, extractedProps []*parser.DestructuringProperty, restElement *parser.DestructuringElement, line int) errors.PaseratiError {
	// Use the new OpCopyObjectExcluding opcode for proper property filtering

	// Create array of property names to exclude
	excludeArrayReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(excludeArrayReg)

	// Count total properties to exclude (both static and computed)
	totalProps := len(extractedProps)

	if totalProps == 0 {
		// No properties to exclude, create empty array and copy object
		c.emitOpCode(vm.OpMakeArray, line)
		c.emitByte(byte(excludeArrayReg))
		c.emitByte(0) // start register (unused for count=0)
		c.emitByte(0) // count: 0 elements
	} else {
		// Allocate contiguous registers for all array elements
		startReg := c.regAlloc.AllocContiguous(totalProps)
		// Mark all registers for cleanup
		for i := 0; i < totalProps; i++ {
			defer c.regAlloc.Free(startReg + Register(i))
		}

		// Load each property key into consecutive registers
		for i, prop := range extractedProps {
			targetReg := startReg + Register(i)
			if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
				// Static identifier key - load as string constant
				nameConstIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
				c.emitLoadConstant(targetReg, nameConstIdx, line)
			} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
				// Computed property key - evaluate expression at runtime
				_, err := c.compileNode(computed.Expr, targetReg)
				if err != nil {
					return err
				}
			}
		}

		// Create array from the element registers
		c.emitOpCode(vm.OpMakeArray, line)
		c.emitByte(byte(excludeArrayReg)) // destination register
		c.emitByte(byte(startReg))        // start register (first element)
		c.emitByte(byte(totalProps))      // element count
	}

	// Create result register for the rest object
	restObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(restObjReg)

	// Use the new opcode: restObj = copyObjectExcluding(sourceObj, excludeArray)
	c.emitOpCode(vm.OpCopyObjectExcluding, line)
	c.emitByte(byte(restObjReg))      // destination register
	c.emitByte(byte(objReg))          // source object register
	c.emitByte(byte(excludeArrayReg)) // exclude array register

	// Assign rest object to the rest property target
	return c.compileSimpleAssignment(restElement.Target, restObjReg, line)
}

// compileObjectRestDeclaration compiles rest property declaration for object destructuring
func (c *Compiler) compileObjectRestDeclaration(objReg Register, extractedProps []*parser.DestructuringProperty, varName string, isConst bool, line int) errors.PaseratiError {
	// Create array of property names to exclude
	excludeArrayReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(excludeArrayReg)

	// Count total properties to exclude (both static and computed)
	totalProps := len(extractedProps)
	if debugAssignment {
		fmt.Printf("// [ObjectRest] Total properties to exclude: %d\n", totalProps)
	}

	// Always use OpCopyObjectExcluding to ensure we only copy enumerable properties
	if totalProps == 0 {
		// Create empty array for exclude list
		c.emitOpCode(vm.OpMakeArray, line)
		c.emitByte(byte(excludeArrayReg))
		c.emitByte(0) // start register (unused for count=0)
		c.emitByte(0) // count: 0 elements
	} else {
		// Allocate contiguous registers for all array elements
		startReg := c.regAlloc.AllocContiguous(totalProps)
		// Mark all registers for cleanup
		for i := 0; i < totalProps; i++ {
			defer c.regAlloc.Free(startReg + Register(i))
		}
		if debugAssignment {
			fmt.Printf("// [ObjectRest] Allocated contiguous registers starting at %d for %d elements\n", startReg, totalProps)
		}

		// Load each property key into consecutive registers
		for i, prop := range extractedProps {
			targetReg := startReg + Register(i)
			if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
				// Static identifier key - load as string constant
				nameConstIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
				c.emitLoadConstant(targetReg, nameConstIdx, line)
				if debugAssignment {
					fmt.Printf("// [ObjectRest] Loading static '%s' into reg %d\n", keyIdent.Value, targetReg)
				}
			} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
				// Computed property key - evaluate expression at runtime
				_, err := c.compileNode(computed.Expr, targetReg)
				if err != nil {
					return err
				}
				if debugAssignment {
					fmt.Printf("// [ObjectRest] Compiled computed property into reg %d\n", targetReg)
				}
			}
		}

		// Create array from the element registers
		c.emitOpCode(vm.OpMakeArray, line)
		c.emitByte(byte(excludeArrayReg)) // destination register
		c.emitByte(byte(startReg))        // start register (first element)
		c.emitByte(byte(totalProps))      // element count
		if debugAssignment {
			fmt.Printf("// [ObjectRest] OpMakeArray: dest=%d, start=%d, count=%d\n", excludeArrayReg, startReg, totalProps)
		}
	}

	// Create result register for the rest object
	restObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(restObjReg)

	// Use OpCopyObjectExcluding which filters out non-enumerable properties
	c.emitOpCode(vm.OpCopyObjectExcluding, line)
	c.emitByte(byte(restObjReg))      // destination register
	c.emitByte(byte(objReg))          // source object register
	c.emitByte(byte(excludeArrayReg)) // exclude array register

	// Define the rest variable with the rest object
	return c.defineDestructuredVariableWithValue(varName, isConst, restObjReg, line)
}

// compileArrayDestructuringDeclaration compiles let/const [a, b] = expr declarations
func (c *Compiler) compileArrayDestructuringDeclaration(node *parser.ArrayDestructuringDeclaration, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	if debugAssignment {
		fmt.Printf("// [Assignment] Compiling array destructuring declaration: %s\n", node.String())
	}

	// If no initializer, assign undefined to all variables
	if node.Value == nil {
		for _, element := range node.Elements {
			if element.Target == nil {
				continue
			}

			if ident, ok := element.Target.(*parser.Identifier); ok {
				// Define variable with undefined value
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Undefined, line)
				if err != nil {
					return BadRegister, err
				}
			}
		}
		return BadRegister, nil
	}

	// 1. Compile RHS expression into temp register
	valueReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(valueReg)

	_, err := c.compileNode(node.Value, valueReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Use iterator protocol for all destructuring
	// This works for arrays (which have Symbol.iterator built-in) AND custom iterables
	// Arrays' Symbol.iterator provides optimal performance by returning numeric indices
	err = c.compileArrayDestructuringIteratorPath(node, valueReg, line)
	if err != nil {
		return BadRegister, err
	}

	return BadRegister, nil
}

// compileArrayDestructuringFastPath compiles array destructuring using numeric indexing (fast path)
func (c *Compiler) compileArrayDestructuringFastPath(node *parser.ArrayDestructuringDeclaration, arrayReg Register, line int) errors.PaseratiError {
	// For each element, compile: define target = array[index]
	for i, element := range node.Elements {
		if element.Target == nil {
			continue // Skip elisions
		}

		var valueReg Register

		if element.IsRest {
			// Rest element: compile array.slice(i) to get remaining elements
			valueReg = c.regAlloc.Alloc()

			// Call array.slice(i) to get the rest of the array
			err := c.compileArraySliceCall(arrayReg, i, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return err
			}
		} else {
			// Regular element: compile array[i]
			indexReg := c.regAlloc.Alloc()
			valueReg = c.regAlloc.Alloc()

			// Load the index as a constant
			indexConstIdx := c.chunk.AddConstant(vm.Number(float64(i)))
			c.emitLoadConstant(indexReg, indexConstIdx, line)

			// Get array[i] using GetIndex operation
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(valueReg)) // destination register
			c.emitByte(byte(arrayReg)) // array register
			c.emitByte(byte(indexReg)) // index register

			c.regAlloc.Free(indexReg)
		}

		// Handle assignment based on target type (identifier vs nested pattern)
		if ident, ok := element.Target.(*parser.Identifier); ok {
			// Simple identifier target
			if element.Default != nil {
				// For const variables, we must compute the value first, then define.
				// If we define first as const, then try to assign, it will fail.
				resultReg := c.regAlloc.Alloc()

				// Check if extracted value is undefined
				jumpToDefault := c.emitPlaceholderJump(vm.OpJumpIfUndefined, valueReg, line)

				// Not undefined - use the extracted value
				c.emitMove(resultReg, valueReg, line)
				jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)

				// Undefined - compile default expression with function name inference
				c.patchJump(jumpToDefault)
				nameHint := ident.Value
				var compileErr errors.PaseratiError

				// Handle function name inference per ECMAScript spec
				if funcLit, ok := element.Default.(*parser.FunctionLiteral); ok && funcLit.Name == nil {
					// Anonymous function literal - use target name
					funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, nameHint)
					if err != nil {
						c.regAlloc.Free(resultReg)
						c.regAlloc.Free(valueReg)
						return err
					}
					c.emitClosure(resultReg, funcConstIndex, funcLit, freeSymbols)
				} else if classExpr, ok := element.Default.(*parser.ClassExpression); ok && classExpr.Name == nil {
					// Anonymous class expression - give it the target name temporarily
					classExpr.Name = &parser.Identifier{
						Token: classExpr.Token,
						Value: nameHint,
					}
					_, compileErr = c.compileNode(classExpr, resultReg)
					classExpr.Name = nil
					if compileErr != nil {
						c.regAlloc.Free(resultReg)
						c.regAlloc.Free(valueReg)
						return compileErr
					}
				} else if arrowFunc, ok := element.Default.(*parser.ArrowFunctionLiteral); ok {
					// Arrow function - compile with name hint
					funcConstIndex, freeSymbols, err := c.compileArrowFunctionWithName(arrowFunc, nameHint)
					if err != nil {
						c.regAlloc.Free(resultReg)
						c.regAlloc.Free(valueReg)
						return err
					}
					var body *parser.BlockStatement
					if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
						body = blockBody
					} else {
						body = &parser.BlockStatement{}
					}
					minimalFuncLit := &parser.FunctionLiteral{Body: body}
					c.emitClosure(resultReg, funcConstIndex, minimalFuncLit, freeSymbols)
				} else {
					// Not a function, compile normally
					_, compileErr = c.compileNode(element.Default, resultReg)
					if compileErr != nil {
						c.regAlloc.Free(resultReg)
						c.regAlloc.Free(valueReg)
						return compileErr
					}
				}

				c.patchJump(jumpPastDefault)

				// Now define the variable with the computed value
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, resultReg, line)
				if err != nil {
					c.regAlloc.Free(resultReg)
					c.regAlloc.Free(valueReg)
					return err
				}

				c.regAlloc.Free(resultReg)
			} else {
				// Define variable with extracted value
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			}
		} else {
			// Nested pattern target (ArrayLiteral or ObjectLiteral)
			if element.Default != nil {
				// Handle conditional assignment for nested patterns
				err := c.compileConditionalAssignmentForDeclaration(element.Target, valueReg, element.Default, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			} else {
				// Direct nested pattern assignment using recursive compilation
				err := c.compileNestedPatternDeclaration(element.Target, valueReg, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			}
		}

		// Clean up temporary registers
		c.regAlloc.Free(valueReg)
	}

	return nil
}

// compileArrayDestructuringIteratorPath compiles array destructuring using iterator protocol
func (c *Compiler) compileArrayDestructuringIteratorPath(node *parser.ArrayDestructuringDeclaration, iterableReg Register, line int) errors.PaseratiError {
	// fmt.Printf("// [COMPILE-ITER] Starting iterator path compilation, iterableReg=R%d\n", iterableReg)

	// Get Symbol.iterator via computed index
	iteratorMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorMethodReg)

	// Load global Symbol
	symbolObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(symbolObjReg)
	symIdx := c.GetOrAssignGlobalIndex("Symbol")
	// fmt.Printf("// [COMPILE-ITER] Getting global Symbol (idx=%d) into R%d\n", symIdx, symbolObjReg)
	c.emitGetGlobal(symbolObjReg, symIdx, line)

	// Get Symbol.iterator
	propNameReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(propNameReg)
	c.emitLoadNewConstant(propNameReg, vm.String("iterator"), line)
	// fmt.Printf("// [COMPILE-ITER] Loading 'iterator' string into R%d\n", propNameReg)

	iteratorKeyReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorKeyReg)
	// fmt.Printf("// [COMPILE-ITER] Getting Symbol.iterator (Symbol[R%d]) into R%d\n", propNameReg, iteratorKeyReg)
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorKeyReg))
	c.emitByte(byte(symbolObjReg))
	c.emitByte(byte(propNameReg))

	// Get iterable[Symbol.iterator]
	// fmt.Printf("// [COMPILE-ITER] Getting iterable[Symbol.iterator] (R%d[R%d]) into R%d\n", iterableReg, iteratorKeyReg, iteratorMethodReg)
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorMethodReg))
	c.emitByte(byte(iterableReg))
	c.emitByte(byte(iteratorKeyReg))

	// Call the iterator method to get iterator object
	iteratorObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorObjReg)
	// fmt.Printf("// [COMPILE-ITER] Calling iterator method R%d on R%d, result in R%d\n", iteratorMethodReg, iterableReg, iteratorObjReg)
	c.emitCallMethod(iteratorObjReg, iteratorMethodReg, iterableReg, 0, line)

	// Allocate register to track iterator.done state
	// We update this each time we call next(), then check it before calling iterator.return()
	doneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(doneReg)
	// Initialize to false
	c.emitLoadFalse(doneReg, line)

	// Track how many elements we've consumed for rest elements
	elementIndex := 0

	// For each element, call iterator.next()
	for _, element := range node.Elements {
		if element.Target == nil {
			// Elision: consume iterator value but don't bind
			c.compileIteratorNext(iteratorObjReg, BadRegister, doneReg, line, true)
			elementIndex++
			continue
		}

		if element.IsRest {
			// Rest element: collect all remaining iterator values into an array
			// Rest elements exhaust the iterator, so we'll update done inside compileIteratorToArray
			restArrayReg := c.regAlloc.Alloc()
			err := c.compileIteratorToArray(iteratorObjReg, restArrayReg, line)
			if err != nil {
				c.regAlloc.Free(restArrayReg)
				return err
			}

			// Bind rest array to target
			if ident, ok := element.Target.(*parser.Identifier); ok {
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, restArrayReg, line)
				c.regAlloc.Free(restArrayReg)
				if err != nil {
					return err
				}
			} else {
				// Nested pattern: [...[x, y]] - destructure the rest array into the pattern
				err := c.compileNestedPatternDeclaration(element.Target, restArrayReg, node.IsConst, line)
				c.regAlloc.Free(restArrayReg)
				if err != nil {
					return err
				}
			}
			// Rest must be last, so we're done - the iterator is exhausted, set done=true
			c.emitLoadTrue(doneReg, line)
			break
		}

		// Regular element: get next value from iterator
		valueReg := c.regAlloc.Alloc()
		c.compileIteratorNext(iteratorObjReg, valueReg, doneReg, line, false)

		// Handle assignment based on target type
		if ident, ok := element.Target.(*parser.Identifier); ok {
			if element.Default != nil {
				// For const variables, we must compute the value first, then define.
				// This avoids the "assignment to const" error since we're not assigning
				// to an already-defined const, we're defining it with a value.
				resultReg := c.regAlloc.Alloc()

				// 1. Conditional jump: if undefined, jump to default value evaluation
				jumpToDefault := c.emitPlaceholderJump(vm.OpJumpIfUndefined, valueReg, line)

				// 2. Path 1: Value is not undefined, copy it to resultReg
				c.emitMove(resultReg, valueReg, line)

				// Jump past the default evaluation
				jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)

				// 3. Path 2: Value is undefined, evaluate default into resultReg with function name inference
				c.patchJump(jumpToDefault)

				nameHint := ident.Value
				var compileErr errors.PaseratiError

				// Handle function name inference per ECMAScript spec
				if funcLit, ok := element.Default.(*parser.FunctionLiteral); ok && funcLit.Name == nil {
					// Anonymous function literal - use target name
					funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, nameHint)
					if err != nil {
						c.patchJump(jumpPastDefault)
						c.regAlloc.Free(resultReg)
						c.regAlloc.Free(valueReg)
						return err
					}
					c.emitClosure(resultReg, funcConstIndex, funcLit, freeSymbols)
				} else if classExpr, ok := element.Default.(*parser.ClassExpression); ok && classExpr.Name == nil {
					// Anonymous class expression - give it the target name temporarily
					classExpr.Name = &parser.Identifier{
						Token: classExpr.Token,
						Value: nameHint,
					}
					_, compileErr = c.compileNode(classExpr, resultReg)
					classExpr.Name = nil
					if compileErr != nil {
						c.patchJump(jumpPastDefault)
						c.regAlloc.Free(resultReg)
						c.regAlloc.Free(valueReg)
						return compileErr
					}
				} else if arrowFunc, ok := element.Default.(*parser.ArrowFunctionLiteral); ok {
					// Arrow function - compile with name hint
					funcConstIndex, freeSymbols, err := c.compileArrowFunctionWithName(arrowFunc, nameHint)
					if err != nil {
						c.patchJump(jumpPastDefault)
						c.regAlloc.Free(resultReg)
						c.regAlloc.Free(valueReg)
						return err
					}
					var body *parser.BlockStatement
					if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
						body = blockBody
					} else {
						body = &parser.BlockStatement{}
					}
					minimalFuncLit := &parser.FunctionLiteral{Body: body}
					c.emitClosure(resultReg, funcConstIndex, minimalFuncLit, freeSymbols)
				} else {
					// Not a function, compile normally
					_, compileErr = c.compileNode(element.Default, resultReg)
					if compileErr != nil {
						c.patchJump(jumpPastDefault)
						c.regAlloc.Free(resultReg)
						c.regAlloc.Free(valueReg)
						return compileErr
					}
				}

				// 4. Patch the jump past default
				c.patchJump(jumpPastDefault)

				// 5. Now resultReg contains the correct value - define the variable with it
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, resultReg, line)
				c.regAlloc.Free(resultReg)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			} else {
				// Define variable with value
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			}
		} else {
			// Nested pattern
			if element.Default != nil {
				err := c.compileConditionalAssignmentForDeclaration(element.Target, valueReg, element.Default, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			} else {
				err := c.compileNestedPatternDeclaration(element.Target, valueReg, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			}
		}

		c.regAlloc.Free(valueReg)
		elementIndex++
	}

	// Call IteratorClose (iterator.return if it exists AND iterator is not done)
	c.emitIteratorCleanupWithDone(iteratorObjReg, doneReg, line)

	return nil
}

// compileObjectDestructuringDeclaration compiles let/const {a, b} = expr declarations
func (c *Compiler) compileObjectDestructuringDeclaration(node *parser.ObjectDestructuringDeclaration, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	if debugAssignment {
		fmt.Printf("// [Assignment] Compiling object destructuring declaration: %s\n", node.String())
	}

	// If no initializer, assign undefined to all variables
	if node.Value == nil {
		for _, prop := range node.Properties {
			if prop.Target == nil {
				continue
			}

			if ident, ok := prop.Target.(*parser.Identifier); ok {
				// Define variable with undefined value
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Undefined, line)
				if err != nil {
					return BadRegister, err
				}
			}
		}

		// Handle rest property without initializer
		if node.RestProperty != nil {
			if ident, ok := node.RestProperty.Target.(*parser.Identifier); ok {
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Undefined, line)
				if err != nil {
					return BadRegister, err
				}
			}
		}

		return BadRegister, nil
	}

	// 1. Compile RHS expression into temp register
	tempReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(tempReg)

	_, err := c.compileNode(node.Value, tempReg)
	if err != nil {
		return BadRegister, err
	}

	// ECMAScript compliance: Throw TypeError if destructuring null or undefined
	// This is required at runtime even if type checker catches it at compile time
	// We need to check: if (tempReg === null || tempReg === undefined) throw TypeError

	if debugAssignment {
		fmt.Printf("// [compileObjectDestructuringDeclaration] Adding null/undefined check for tempReg=R%d\n", tempReg)
	}

	// Allocate register for null/undefined checks
	checkReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(checkReg)

	// Check if tempReg is null (use strict equality so undefined !== null)
	nullConstIdx := c.chunk.AddConstant(vm.Null)
	c.emitLoadConstant(checkReg, nullConstIdx, line)
	c.emitOpCode(vm.OpStrictEqual, line)
	c.emitByte(byte(checkReg)) // result register
	c.emitByte(byte(tempReg))  // left operand
	c.emitByte(byte(checkReg)) // right operand (null)

	// Jump past error if not null
	notNullJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, checkReg, line)

	// Throw TypeError: Cannot destructure null
	errorReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(errorReg)
	typeErrorGlobalIdx := c.GetOrAssignGlobalIndex("TypeError")
	c.emitGetGlobal(errorReg, typeErrorGlobalIdx, line)

	msgReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(msgReg)
	msgConstIdx := c.chunk.AddConstant(vm.String("Cannot destructure 'null'"))
	c.emitLoadConstant(msgReg, msgConstIdx, line)

	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	c.emitCall(resultReg, errorReg, 1, line) // Call TypeError constructor with message
	c.emitOpCode(vm.OpThrow, line)
	c.emitByte(byte(resultReg))

	// Patch jump for not-null case
	c.patchJump(notNullJump)

	// Check if tempReg is undefined (use strict equality so null !== undefined)
	undefConstIdx := c.chunk.AddConstant(vm.Undefined)
	c.emitLoadConstant(checkReg, undefConstIdx, line)
	c.emitOpCode(vm.OpStrictEqual, line)
	c.emitByte(byte(checkReg)) // result register
	c.emitByte(byte(tempReg))  // left operand
	c.emitByte(byte(checkReg)) // right operand (undefined)

	// Jump past error if not undefined
	notUndefJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, checkReg, line)

	// Throw TypeError: Cannot destructure undefined
	c.emitGetGlobal(errorReg, typeErrorGlobalIdx, line)
	msgConstIdx = c.chunk.AddConstant(vm.String("Cannot destructure 'undefined'"))
	c.emitLoadConstant(msgReg, msgConstIdx, line)
	c.emitCall(resultReg, errorReg, 1, line) // Call TypeError constructor with message
	c.emitOpCode(vm.OpThrow, line)
	c.emitByte(byte(resultReg))

	// Patch jump for not-undefined case
	c.patchJump(notUndefJump)

	// 2. For each property, compile: define target = temp.property
	for _, prop := range node.Properties {
		if prop.Key == nil || prop.Target == nil {
			continue // Skip malformed properties
		}

		// Support both identifier and nested pattern targets

		// Allocate register for extracted value
		valueReg := c.regAlloc.Alloc()

		// Handle property access (identifier or computed)
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			// Check if the property name is numeric (for array index access)
			// This handles cases like {0: x, 1: y} destructuring from arrays
			isNumeric := false
			for _, ch := range keyIdent.Value {
				if ch < '0' || ch > '9' {
					isNumeric = false
					break
				}
				isNumeric = true
			}

			if isNumeric && len(keyIdent.Value) > 0 {
				// Use OpGetIndex for numeric properties (array elements)
				// Convert string to number for proper array indexing
				var indexNum float64
				_, _ = fmt.Sscanf(keyIdent.Value, "%f", &indexNum)
				indexConstIdx := c.chunk.AddConstant(vm.Number(indexNum))
				indexReg := c.regAlloc.Alloc()
				c.emitLoadConstant(indexReg, indexConstIdx, line)
				c.emitOpCode(vm.OpGetIndex, line)
				c.emitByte(byte(valueReg))
				c.emitByte(byte(tempReg))
				c.emitByte(byte(indexReg))
				c.regAlloc.Free(indexReg)
			} else {
				// Use OpGetProp for regular string properties
				propNameIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
				c.emitOpCode(vm.OpGetProp, line)
				c.emitByte(byte(valueReg)) // destination register
				c.emitByte(byte(tempReg))  // object register
				c.emitUint16(propNameIdx)  // property name constant index
			}
		} else if numLit, ok := prop.Key.(*parser.NumberLiteral); ok {
			// Number literal key: use OpGetIndex for numeric property access
			// This allows destructuring arrays by numeric index: {0: v, 1: w} = [7, 8]
			indexConstIdx := c.chunk.AddConstant(vm.Number(numLit.Value))
			indexReg := c.regAlloc.Alloc()
			c.emitLoadConstant(indexReg, indexConstIdx, line)
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(valueReg))
			c.emitByte(byte(tempReg))
			c.emitByte(byte(indexReg))
			c.regAlloc.Free(indexReg)
		} else if bigIntLit, ok := prop.Key.(*parser.BigIntLiteral); ok {
			// BigInt literal key: convert to string property name (numeric part without 'n')
			propName := bigIntLit.Value
			propNameIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitOpCode(vm.OpGetProp, line)
			c.emitByte(byte(valueReg)) // destination register
			c.emitByte(byte(tempReg))  // object register
			c.emitUint16(propNameIdx)  // property name constant index
		} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
			keyReg := c.regAlloc.Alloc()
			_, err := c.compileNode(computed.Expr, keyReg)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(keyReg)
				return BadRegister, err
			}
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(valueReg))
			c.emitByte(byte(tempReg))
			c.emitByte(byte(keyReg))
			c.regAlloc.Free(keyReg)
		}

		// Handle assignment based on target type (identifier vs nested pattern)
		if ident, ok := prop.Target.(*parser.Identifier); ok {
			// Simple identifier target
			if prop.Default != nil {
				// First, define the variable to reserve the name and get the target register
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Any, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}

				// Get the target identifier for conditional assignment
				targetIdent := &parser.Identifier{
					Token: ident.Token,
					Value: ident.Value,
				}

				// Use conditional assignment: target = valueReg !== undefined ? valueReg : defaultExpr
				err = c.compileConditionalAssignment(targetIdent, valueReg, prop.Default, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
			} else {
				// Define variable with extracted value
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
			}
		} else {
			// Nested pattern target (ArrayLiteral or ObjectLiteral)
			if prop.Default != nil {
				// Handle conditional assignment for nested patterns
				err := c.compileConditionalAssignmentForDeclaration(prop.Target, valueReg, prop.Default, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
			} else {
				// Direct nested pattern assignment using recursive compilation
				err := c.compileNestedPatternDeclaration(prop.Target, valueReg, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
			}
		}

		// Clean up temporary register
		c.regAlloc.Free(valueReg)
	}

	// Handle rest property if present
	if node.RestProperty != nil {
		if ident, ok := node.RestProperty.Target.(*parser.Identifier); ok {
			// Create rest object with remaining properties
			err := c.compileObjectRestDeclaration(tempReg, node.Properties, ident.Value, node.IsConst, line)
			if err != nil {
				return BadRegister, err
			}
		}
	}

	return BadRegister, nil
}

// defineDestructuredVariable defines a new variable from destructuring (without value)
func (c *Compiler) defineDestructuredVariable(name string, isConst bool, valueType types.Type, line int) errors.PaseratiError {
	undefReg := c.regAlloc.Alloc()
	c.emitLoadUndefined(undefReg, line)

	err := c.defineDestructuredVariableWithValue(name, isConst, undefReg, line)
	if err != nil {
		c.regAlloc.Free(undefReg)
		return err
	}

	// Pin the register for local variables
	if c.enclosing != nil {
		c.regAlloc.Pin(undefReg)
	}

	return nil
}

// defineDestructuredVariableWithValue defines a new variable from destructuring with a specific value
func (c *Compiler) defineDestructuredVariableWithValue(name string, isConst bool, valueReg Register, line int) errors.PaseratiError {
	// Check if we're truly at global scope: no enclosing function AND no enclosed symbol table
	// For loops with let/const create an enclosed symbol table, so those variables should be local
	isGlobalScope := c.enclosing == nil && c.currentSymbolTable.Outer == nil
	if isGlobalScope {
		// Top-level: use global variable
		globalIdx := c.GetOrAssignGlobalIndex(name)
		c.emitSetGlobalInit(globalIdx, valueReg, line)
		c.currentSymbolTable.DefineGlobal(name, globalIdx)
	} else {
		// IMPORTANT: Check ONLY the CURRENT scope for existing bindings.
		// This is critical for proper shadowing (e.g., catch parameters should shadow outer vars).
		// Variables from outer scopes should NOT be reused - let/const creates a new binding.
		if sym, exists := c.currentSymbolTable.store[name]; exists {
			// Variable already exists in current scope (e.g., from hoisting or pre-definition)
			if sym.Register != nilRegister {
				if valueReg != sym.Register {
					c.emitMove(sym.Register, valueReg, line)
				}
			} else if sym.IsSpilled {
				c.emitStoreSpill(sym.SpillIndex, valueReg, line)
			}
			// Don't redefine - keep existing symbol table entry
		} else {
			// Variable doesn't exist in current scope - define it.
			// This creates a new binding that shadows any outer scope variables.
			c.currentSymbolTable.Define(name, valueReg)
			// Pin the register since local variables can be captured by upvalues
			c.regAlloc.Pin(valueReg)
		}
	}

	// Mark TDZ as initialized now that the destructuring variable has been assigned
	c.currentSymbolTable.InitializeTDZ(name)

	return nil
}

// compileAssignmentToMember compiles assignment to a member expression: obj.prop = valueReg or obj[key] = valueReg
func (c *Compiler) compileAssignmentToMember(memberExpr *parser.MemberExpression, valueReg Register, line int) errors.PaseratiError {
	// Compile the object expression
	objectReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(objectReg)

	_, err := c.compileNode(memberExpr.Object, objectReg)
	if err != nil {
		return err
	}

	// Check if this is a computed property
	if computedKey, ok := memberExpr.Property.(*parser.ComputedPropertyName); ok {
		// Computed property: obj[expr] = value
		keyReg := c.regAlloc.Alloc()
		defer c.regAlloc.Free(keyReg)

		_, err := c.compileNode(computedKey.Expr, keyReg)
		if err != nil {
			return err
		}

		// Emit OpSetIndex: objectReg[keyReg] = valueReg
		c.emitOpCode(vm.OpSetIndex, line)
		c.emitByte(byte(objectReg))
		c.emitByte(byte(keyReg))
		c.emitByte(byte(valueReg))
	} else {
		// Regular property: obj.prop = value
		propName := c.extractPropertyName(memberExpr.Property)

		// Check for private field (starts with #)
		if len(propName) > 0 && propName[0] == '#' {
			// Private field
			fieldName := propName[1:]
			// Use branded key to distinguish private fields with same name in different classes
			brandedKey := c.getPrivateFieldKey(fieldName)
			nameConstIdx := c.chunk.AddConstant(vm.String(brandedKey))

			// Check if this member is declared as a setter
			if kind, _, ok := c.getPrivateMemberKind(fieldName); ok && (kind == PrivateMemberSetter || kind == PrivateMemberAccessor) {
				c.emitCallPrivateSetter(objectReg, valueReg, nameConstIdx, line)
			} else {
				c.emitSetPrivateField(objectReg, valueReg, nameConstIdx, line)
			}
		} else {
			// Regular property
			nameConstIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitSetProp(objectReg, valueReg, nameConstIdx, line)
		}
	}

	return nil
}

// compileAssignmentToIndex compiles assignment to an index expression: obj[key] = valueReg
func (c *Compiler) compileAssignmentToIndex(indexExpr *parser.IndexExpression, valueReg Register, line int) errors.PaseratiError {
	// Compile the object/array expression
	objectReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(objectReg)

	_, err := c.compileNode(indexExpr.Left, objectReg)
	if err != nil {
		return err
	}

	// Compile the index expression
	keyReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(keyReg)

	_, err = c.compileNode(indexExpr.Index, keyReg)
	if err != nil {
		return err
	}

	// Emit OpSetIndex: objectReg[keyReg] = valueReg
	c.emitOpCode(vm.OpSetIndex, line)
	c.emitByte(byte(objectReg))
	c.emitByte(byte(keyReg))
	c.emitByte(byte(valueReg))

	return nil
}

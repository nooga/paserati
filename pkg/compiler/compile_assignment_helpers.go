package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// DestructuringTargetRef holds pre-evaluated reference information for a destructuring target.
// Per ECMAScript spec 13.15.5.6, the target reference must be evaluated BEFORE calling
// iterator.next(), so we evaluate the reference first, store these registers, then
// call next(), and finally complete the assignment.
type DestructuringTargetRef struct {
	// Type indicates what kind of target this is
	Type DestructuringTargetType

	// For Identifier targets - symbol info (no registers needed, resolved at compile time)
	Symbol     *Symbol
	SymbolName string
	IsGlobal   bool
	GlobalIdx  uint16

	// For MemberExpression targets (obj.prop)
	ObjReg  Register // Pre-evaluated object
	PropIdx uint16   // Constant index for property name

	// For IndexExpression targets (obj[index])
	IndexReg Register // Pre-evaluated index

	// For nested patterns (ArrayLiteral or ObjectLiteral) - these need recursive handling
	NestedPattern parser.Expression
}

type DestructuringTargetType int

const (
	TargetTypeIdentifier DestructuringTargetType = iota
	TargetTypeMember
	TargetTypeIndex
	TargetTypeNestedArray
	TargetTypeNestedObject
	TargetTypeElision // For holes in array patterns like [,]
)

// compileDestructuringTargetRef evaluates the target reference and returns the pre-evaluated info.
// This should be called BEFORE iterator.next() per ECMAScript spec.
// The caller is responsible for freeing the allocated registers after assignment is complete.
func (c *Compiler) compileDestructuringTargetRef(target parser.Expression, line int) (*DestructuringTargetRef, errors.PaseratiError) {
	switch targetNode := target.(type) {
	case *parser.Identifier:
		// For identifiers, resolve at compile time - no runtime evaluation needed before next()
		ref := &DestructuringTargetRef{Type: TargetTypeIdentifier, SymbolName: targetNode.Value}

		// Check if this is from a with object
		if targetNode.IsFromWith {
			if objReg, withFound := c.currentSymbolTable.ResolveWithProperty(targetNode.Value); withFound {
				// Treat as member expression on with object
				ref.Type = TargetTypeMember
				ref.ObjReg = objReg
				ref.PropIdx = c.chunk.AddConstant(vm.String(targetNode.Value))
				return ref, nil
			}
		}

		symbol, _, found := c.currentSymbolTable.Resolve(targetNode.Value)
		if !found {
			// In strict mode, this is a ReferenceError
			// In non-strict mode, it's an implicit global
			if c.chunk.IsStrict {
				ref.IsGlobal = false // Will emit error at assignment time
			} else {
				ref.IsGlobal = true
				ref.GlobalIdx = c.GetOrAssignGlobalIndex(targetNode.Value)
			}
		} else {
			ref.Symbol = &symbol
			ref.IsGlobal = symbol.IsGlobal
			if symbol.IsGlobal {
				ref.GlobalIdx = symbol.GlobalIndex
			}
		}
		return ref, nil

	case *parser.MemberExpression:
		// Evaluate the object expression first
		objReg := c.regAlloc.Alloc()
		_, err := c.compileNode(targetNode.Object, objReg)
		if err != nil {
			c.regAlloc.Free(objReg)
			return nil, err
		}

		propName, ok := targetNode.Property.(*parser.Identifier)
		if !ok {
			c.regAlloc.Free(objReg)
			return nil, NewCompileError(targetNode, "member expression property must be an identifier")
		}

		return &DestructuringTargetRef{
			Type:    TargetTypeMember,
			ObjReg:  objReg,
			PropIdx: c.chunk.AddConstant(vm.String(propName.Value)),
		}, nil

	case *parser.IndexExpression:
		// Evaluate both object and index expressions
		objReg := c.regAlloc.Alloc()
		_, err := c.compileNode(targetNode.Left, objReg)
		if err != nil {
			c.regAlloc.Free(objReg)
			return nil, err
		}

		indexReg := c.regAlloc.Alloc()
		_, err = c.compileNode(targetNode.Index, indexReg)
		if err != nil {
			c.regAlloc.Free(objReg)
			c.regAlloc.Free(indexReg)
			return nil, err
		}

		return &DestructuringTargetRef{
			Type:     TargetTypeIndex,
			ObjReg:   objReg,
			IndexReg: indexReg,
		}, nil

	case *parser.ArrayLiteral:
		// Nested array pattern - will need recursive handling
		return &DestructuringTargetRef{
			Type:          TargetTypeNestedArray,
			NestedPattern: targetNode,
		}, nil

	case *parser.ObjectLiteral:
		// Nested object pattern - will need recursive handling
		return &DestructuringTargetRef{
			Type:          TargetTypeNestedObject,
			NestedPattern: targetNode,
		}, nil

	case *parser.UndefinedLiteral:
		// Elision/hole in array pattern like [,] - no assignment needed, just consume iterator value
		return &DestructuringTargetRef{
			Type: TargetTypeElision,
		}, nil

	default:
		return nil, NewCompileError(target, fmt.Sprintf("unsupported destructuring target type: %T", target))
	}
}

// assignToDestructuringTargetRef completes the assignment using pre-evaluated reference info.
// This should be called AFTER iterator.next() returns the value to assign.
func (c *Compiler) assignToDestructuringTargetRef(ref *DestructuringTargetRef, valueReg Register, line int) errors.PaseratiError {
	switch ref.Type {
	case TargetTypeIdentifier:
		if ref.Symbol != nil {
			// Check for const/immutable assignment
			if ref.Symbol.IsConst {
				c.emitConstAssignmentError(ref.SymbolName, line)
				return nil
			}
			if ref.Symbol.IsStrictImmutable {
				c.emitConstAssignmentError(ref.SymbolName, line)
				return nil
			}
			if ref.Symbol.IsImmutable && c.chunk.IsStrict {
				c.emitConstAssignmentError(ref.SymbolName, line)
				return nil
			}

			if ref.Symbol.IsGlobal {
				c.emitSetGlobal(ref.Symbol.GlobalIndex, valueReg, line)
			} else if ref.Symbol.IsSpilled {
				c.emitStoreSpill(ref.Symbol.SpillIndex, valueReg, line)
			} else {
				if valueReg != ref.Symbol.Register {
					c.emitMove(ref.Symbol.Register, valueReg, line)
				}
			}
		} else if ref.IsGlobal {
			c.emitSetGlobal(ref.GlobalIdx, valueReg, line)
		} else {
			// Strict mode unresolvable reference
			c.emitStrictUnresolvableReferenceError(ref.SymbolName, line)
		}
		return nil

	case TargetTypeMember:
		c.emitSetProp(ref.ObjReg, valueReg, ref.PropIdx, line)
		return nil

	case TargetTypeIndex:
		c.emitOpCode(vm.OpSetIndex, line)
		c.emitByte(byte(ref.ObjReg))
		c.emitByte(byte(ref.IndexReg))
		c.emitByte(byte(valueReg))
		return nil

	case TargetTypeNestedArray:
		return c.compileNestedArrayDestructuring(ref.NestedPattern.(*parser.ArrayLiteral), valueReg, line)

	case TargetTypeNestedObject:
		return c.compileNestedObjectDestructuring(ref.NestedPattern.(*parser.ObjectLiteral), valueReg, line)

	case TargetTypeElision:
		// Elision - no assignment needed, value is discarded
		return nil

	default:
		return NewCompileError(nil, fmt.Sprintf("unknown target ref type: %d", ref.Type))
	}
}

// freeDestructuringTargetRef releases any registers allocated for the target reference.
func (c *Compiler) freeDestructuringTargetRef(ref *DestructuringTargetRef) {
	switch ref.Type {
	case TargetTypeMember:
		c.regAlloc.Free(ref.ObjReg)
	case TargetTypeIndex:
		c.regAlloc.Free(ref.ObjReg)
		c.regAlloc.Free(ref.IndexReg)
	}
	// Identifier and nested patterns don't allocate registers that need freeing
}

// compileConditionalAssignmentWithTargetRef handles default value assignment with a pre-evaluated target ref.
// This implements: target = value !== undefined ? value : defaultExpr
// The target reference has already been evaluated, so we just need to:
// 1. Check if value is undefined
// 2. If so, compile and use the default expression (with function name inference if applicable)
// 3. Assign using the pre-evaluated target reference
func (c *Compiler) compileConditionalAssignmentWithTargetRef(ref *DestructuringTargetRef, valueReg Register, defaultExpr parser.Expression, line int) errors.PaseratiError {
	// Use OpJumpIfUndefined for efficiency (matches compileConditionalAssignment)
	jumpToDefault := c.emitPlaceholderJump(vm.OpJumpIfUndefined, valueReg, line)

	// Path 1: Value is not undefined, assign valueReg to target
	err := c.assignToDestructuringTargetRef(ref, valueReg, line)
	if err != nil {
		c.patchJump(jumpToDefault)
		return err
	}

	// Jump past the default assignment
	jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)

	// Path 2: Value is undefined, evaluate and assign default
	c.patchJump(jumpToDefault)

	// Compile the default expression
	defaultReg := c.regAlloc.Alloc()

	// Check if we should apply function name inference
	// Per ECMAScript spec: if target is an identifier and default is anonymous function, use target name
	var nameHint string
	if ref.Type == TargetTypeIdentifier && ref.SymbolName != "" {
		nameHint = ref.SymbolName
	}

	// Compile default with potential name hint for anonymous functions
	if nameHint != "" {
		if funcLit, ok := defaultExpr.(*parser.FunctionLiteral); ok && funcLit.Name == nil {
			// Anonymous function literal - use target name
			funcConstIndex, freeSymbols, compileErr := c.compileFunctionLiteral(funcLit, nameHint)
			if compileErr != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return compileErr
			}
			c.emitClosure(defaultReg, funcConstIndex, funcLit, freeSymbols)
		} else if classExpr, ok := defaultExpr.(*parser.ClassExpression); ok && classExpr.Name == nil {
			// Anonymous class expression - give it the target name temporarily with inferred prefix
			classExpr.Name = &parser.Identifier{
				Token: classExpr.Token,
				Value: "__Inferred__" + nameHint,
			}
			_, compileErr := c.compileNode(classExpr, defaultReg)
			classExpr.Name = nil
			if compileErr != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return compileErr
			}
		} else if arrowFunc, ok := defaultExpr.(*parser.ArrowFunctionLiteral); ok {
			// Arrow function - compile with name hint
			funcConstIndex, freeSymbols, compileErr := c.compileArrowFunctionWithName(arrowFunc, nameHint)
			if compileErr != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return compileErr
			}
			var body *parser.BlockStatement
			if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
				body = blockBody
			} else {
				body = &parser.BlockStatement{}
			}
			minimalFuncLit := &parser.FunctionLiteral{Body: body}
			c.emitClosure(defaultReg, funcConstIndex, minimalFuncLit, freeSymbols)
		} else {
			// Not a function, compile normally
			_, compileErr := c.compileNode(defaultExpr, defaultReg)
			if compileErr != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return compileErr
			}
		}
	} else {
		_, compileErr := c.compileNode(defaultExpr, defaultReg)
		if compileErr != nil {
			c.patchJump(jumpPastDefault)
			c.regAlloc.Free(defaultReg)
			return compileErr
		}
	}

	// Assign default value to target
	err = c.assignToDestructuringTargetRef(ref, defaultReg, line)
	c.regAlloc.Free(defaultReg)
	if err != nil {
		c.patchJump(jumpPastDefault)
		return err
	}

	// Patch skip jump
	c.patchJump(jumpPastDefault)

	return nil
}

// emitDestructuringNullCheck emits bytecode to check if valueReg is null or undefined
// and throws TypeError if so. This is required by ECMAScript for destructuring operations.
//
// The check is done at runtime even if type checker catches it at compile time,
// because JavaScript allows null/undefined to be passed despite type annotations.
func (c *Compiler) emitDestructuringNullCheck(valueReg Register, line int) {
	if debugCompiler {
		fmt.Printf("// [emitDestructuringNullCheck] Emitting null/undefined check for R%d\n", valueReg)
	}

	// Allocate register for null/undefined checks
	checkReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(checkReg)

	// Check if valueReg is null (use strict equality so undefined !== null)
	nullConstIdx := c.chunk.AddConstant(vm.Null)
	c.emitLoadConstant(checkReg, nullConstIdx, line)
	c.emitOpCode(vm.OpStrictEqual, line)
	c.emitByte(byte(checkReg)) // result register
	c.emitByte(byte(valueReg)) // left operand
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

	// Check if valueReg is undefined (use strict equality so null !== undefined)
	undefConstIdx := c.chunk.AddConstant(vm.Undefined)
	c.emitLoadConstant(checkReg, undefConstIdx, line)
	c.emitOpCode(vm.OpStrictEqual, line)
	c.emitByte(byte(checkReg)) // result register
	c.emitByte(byte(valueReg)) // left operand
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
}

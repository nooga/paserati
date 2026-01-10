package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// compileEnumDeclaration compiles an enum declaration to bytecode
func (c *Compiler) compileEnumDeclaration(node *parser.EnumDeclaration, hint Register) (Register, errors.PaseratiError) {
	debugPrintf("// [Compiler Enum] Compiling enum declaration '%s'\n", node.Name.Value)

	// Create the enum object with both forward and reverse mappings
	enumObj := vm.NewDictObject(vm.DefaultObjectPrototype)
	enumDict := enumObj.AsDictObject()

	// Track next auto-increment value for numeric enums
	nextValue := 0
	isNumeric := true
	lastMemberWasNumeric := true // Track if the previous member was numeric

	// Process enum members
	for _, member := range node.Members {
		memberName := member.Name.Value
		var memberValue vm.Value
		var isThisMemberNumeric bool

		if member.Value != nil {

			// For compile-time constant evaluation, we need to check the AST node type
			switch v := member.Value.(type) {
			case *parser.NumberLiteral:
				memberValue = vm.Number(v.Value)
				nextValue = int(v.Value) + 1
				isThisMemberNumeric = true
			case *parser.StringLiteral:
				memberValue = vm.String(v.Value)
				isNumeric = false
				isThisMemberNumeric = false
			case *parser.PrefixExpression:
				// Handle negative numbers
				if v.Operator == "-" {
					if numLit, ok := v.Right.(*parser.NumberLiteral); ok {
						memberValue = vm.Number(-numLit.Value)
						nextValue = int(-numLit.Value) + 1
						isThisMemberNumeric = true
					} else {
						return BadRegister, NewCompileError(member.Value, "enum member must have constant initializer")
					}
				} else {
					return BadRegister, NewCompileError(member.Value, "enum member must have constant initializer")
				}
			default:
				// For complex expressions, we would need runtime evaluation
				// For now, only support compile-time constants
				return BadRegister, NewCompileError(member.Value, "enum member initializer must be a constant expression")
			}
		} else {
			// No initializer - use auto-increment
			if !lastMemberWasNumeric {
				return BadRegister, NewCompileError(member.Name, "enum member must have initializer")
			}
			memberValue = vm.Number(float64(nextValue))
			nextValue++
			isThisMemberNumeric = true
		}

		// Add forward mapping: memberName -> value
		enumDict.SetOwn(memberName, memberValue)

		// Add reverse mapping for numeric enums: value -> memberName
		if isNumeric {
			if memberValue.IsNumber() {
				if memberValue.IsIntegerNumber() {
					indexKey := fmt.Sprintf("%d", memberValue.AsInteger())
					enumDict.SetOwn(indexKey, vm.String(memberName))
				} else {
					indexKey := fmt.Sprintf("%.0f", memberValue.AsFloat())
					enumDict.SetOwn(indexKey, vm.String(memberName))
				}
			}
		}

		debugPrintf("// [Compiler Enum] Added member '%s' = %s\n", memberName, memberValue.ToString())

		// Update tracking for next iteration
		lastMemberWasNumeric = isThisMemberNumeric
	}

	// Store the enum object as a constant and load it into a register
	enumConstIndex := c.chunk.AddConstant(enumObj)
	c.emitLoadConstant(hint, enumConstIndex, node.Token.Line)

	// Define the enum as a global symbol (like function declarations)
	globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
	c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
	c.emitSetGlobal(globalIdx, hint, node.Token.Line)

	debugPrintf("// [Compiler Enum] Defined global enum '%s' at global index %d\n", node.Name.Value, globalIdx)

	debugPrintf("// [Compiler Enum] Successfully compiled enum '%s' with %d members\n",
		node.Name.Value, len(node.Members))

	return hint, nil
}

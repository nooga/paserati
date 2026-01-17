package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// compileNestedPatternDeclaration handles nested pattern variable declarations
func (c *Compiler) compileNestedPatternDeclaration(target parser.Expression, valueReg Register, isConst bool, line int) errors.PaseratiError {
	switch targetNode := target.(type) {
	case *parser.ArrayLiteral:
		// Convert to ArrayDestructuringDeclaration and compile
		return c.compileNestedArrayDeclaration(targetNode, valueReg, isConst, line)
	case *parser.ObjectLiteral:
		// Convert to ObjectDestructuringDeclaration and compile
		return c.compileNestedObjectDeclaration(targetNode, valueReg, isConst, line)
	case *parser.ArrayParameterPattern:
		// Handle ArrayParameterPattern from transformed function parameters
		return c.compileNestedArrayParameterPattern(targetNode, valueReg, isConst, line)
	case *parser.ObjectParameterPattern:
		// Handle ObjectParameterPattern from transformed function parameters
		return c.compileNestedObjectParameterPattern(targetNode, valueReg, isConst, line)
	case *parser.UndefinedLiteral:
		// Elision in destructuring - no code to generate, just skip this element
		return nil
	default:
		return NewCompileError(target, fmt.Sprintf("unsupported nested pattern type: %T", target))
	}
}

// compileNestedArrayDeclaration handles nested array pattern declarations
func (c *Compiler) compileNestedArrayDeclaration(arrayTarget *parser.ArrayLiteral, valueReg Register, isConst bool, line int) errors.PaseratiError {
	// Convert ArrayLiteral to ArrayDestructuringDeclaration format
	declaration := &parser.ArrayDestructuringDeclaration{
		Token:   arrayTarget.Token,
		IsConst: isConst,
		Value:   nil, // We already have the value in valueReg
	}

	// Convert elements to destructuring elements
	for i, element := range arrayTarget.Elements {
		var target parser.Expression
		var defaultValue parser.Expression
		var isRest bool

		// Check if this element is a rest element (...rest)
		if spreadExpr, ok := element.(*parser.SpreadElement); ok {
			target = spreadExpr.Argument
			defaultValue = nil
			isRest = true
		} else if assignExpr, ok := element.(*parser.AssignmentExpression); ok && assignExpr.Operator == "=" {
			target = assignExpr.Left
			defaultValue = assignExpr.Value
			isRest = false
		} else {
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
		if isRest && i != len(arrayTarget.Elements)-1 {
			return NewCompileError(arrayTarget, "rest element must be last element in destructuring pattern")
		}

		declaration.Elements = append(declaration.Elements, destElement)
	}

	// Use iterator protocol for nested destructuring (handles arrays and iterables)
	return c.compileArrayDestructuringIteratorPath(declaration, valueReg, line)
}

// compileNestedObjectDeclaration handles nested object pattern declarations
func (c *Compiler) compileNestedObjectDeclaration(objectTarget *parser.ObjectLiteral, valueReg Register, isConst bool, line int) errors.PaseratiError {
	// Convert ObjectLiteral to ObjectDestructuringDeclaration format
	declaration := &parser.ObjectDestructuringDeclaration{
		Token:   objectTarget.Token,
		IsConst: isConst,
		Value:   nil, // We already have the value in valueReg
	}

	// Convert properties to destructuring properties
	for _, prop := range objectTarget.Properties {
		// Key can be identifier, number, or bigint (for array destructuring as object)
		var keyIdent *parser.Identifier
		if ident, ok := prop.Key.(*parser.Identifier); ok {
			keyIdent = ident
		} else if numLit, ok := prop.Key.(*parser.NumberLiteral); ok {
			// Convert number to identifier for destructuring property
			keyIdent = &parser.Identifier{Token: numLit.Token, Value: numLit.Token.Literal}
		} else if bigIntLit, ok := prop.Key.(*parser.BigIntLiteral); ok {
			// Convert bigint to identifier for destructuring property (using numeric part without 'n')
			keyIdent = &parser.Identifier{Token: bigIntLit.Token, Value: bigIntLit.Value}
		} else {
			return NewCompileError(objectTarget, fmt.Sprintf("invalid destructuring property key: %s (only identifiers, numbers, and bigints supported)", prop.Key.String()))
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

		if valueIdent, ok := prop.Value.(*parser.Identifier); ok && valueIdent.Value == keyIdent.Value {
			// Pattern 1: Shorthand without default {name}
			target = keyIdent
			defaultValue = nil
		} else if assignExpr, ok := prop.Value.(*parser.AssignmentExpression); ok && assignExpr.Operator == "=" {
			// Check if this is shorthand with default or explicit with default
			if leftIdent, ok := assignExpr.Left.(*parser.Identifier); ok && leftIdent.Value == keyIdent.Value {
				// Pattern 2: Shorthand with default {name = defaultVal}
				target = keyIdent
				defaultValue = assignExpr.Value
			} else {
				// Pattern 4: Explicit target with default {name: localVar = defaultVal}
				target = assignExpr.Left
				defaultValue = assignExpr.Value
			}
		} else {
			// Pattern 3, 5, 6: Explicit target without default {name: localVar} or {name: [a, b]} or {name: {x, y}}
			target = prop.Value
			defaultValue = nil
		}

		destProperty := &parser.DestructuringProperty{
			Key:     keyIdent,
			Target:  target,
			Default: defaultValue,
		}

		declaration.Properties = append(declaration.Properties, destProperty)
	}

	// Reuse existing compilation logic but with direct value register
	return c.compileObjectDestructuringDeclarationWithValueReg(declaration, valueReg, line)
}

// compileArrayDestructuringDeclarationWithValueReg compiles array destructuring declarations using an existing value register
func (c *Compiler) compileArrayDestructuringDeclarationWithValueReg(node *parser.ArrayDestructuringDeclaration, valueReg Register, line int) errors.PaseratiError {
	// ECMAScript compliance: Throw TypeError if destructuring null or undefined
	// This is required at runtime even if type checker catches it at compile time
	c.emitDestructuringNullCheck(valueReg, line)

	// Reuse existing array destructuring logic but skip RHS compilation
	// For each element, compile: define target = valueReg[index]
	for i, element := range node.Elements {
		if element.Target == nil {
			continue // Skip malformed elements
		}

		var extractedReg Register

		if element.IsRest {
			// Rest element: compile valueReg.slice(i) to get remaining elements
			extractedReg = c.regAlloc.Alloc()

			// Call valueReg.slice(i) to get the rest of the array
			err := c.compileArraySliceCall(valueReg, i, extractedReg, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		} else {
			// Regular element: compile valueReg[i]
			indexReg := c.regAlloc.Alloc()
			extractedReg = c.regAlloc.Alloc()

			// Load the index as a constant
			indexConstIdx := c.chunk.AddConstant(vm.Number(float64(i)))
			c.emitLoadConstant(indexReg, indexConstIdx, line)

			// Get valueReg[i] using GetIndex operation
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(extractedReg)) // destination register
			c.emitByte(byte(valueReg))     // array register
			c.emitByte(byte(indexReg))     // index register

			c.regAlloc.Free(indexReg)
		}

		// Handle assignment based on target type (identifier vs nested pattern)
		if ident, ok := element.Target.(*parser.Identifier); ok {
			// Simple identifier target
			if element.Default != nil {
				// First, define the variable to reserve the name and get the target register
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Any, line)
				if err != nil {
					c.regAlloc.Free(extractedReg)
					return err
				}

				// Get the target identifier for conditional assignment
				targetIdent := &parser.Identifier{
					Token: ident.Token,
					Value: ident.Value,
				}

				// Use conditional assignment: target = extractedReg !== undefined ? extractedReg : defaultExpr
				err = c.compileConditionalAssignment(targetIdent, extractedReg, element.Default, line)
				if err != nil {
					c.regAlloc.Free(extractedReg)
					return err
				}
			} else {
				// Define variable with extracted value
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, extractedReg, line)
				if err != nil {
					c.regAlloc.Free(extractedReg)
					return err
				}
			}
		} else {
			// Nested pattern target (ArrayLiteral or ObjectLiteral)
			if element.Default != nil {
				// Handle conditional assignment for nested patterns
				err := c.compileConditionalAssignmentForDeclaration(element.Target, extractedReg, element.Default, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(extractedReg)
					return err
				}
			} else {
				// Direct nested pattern assignment using recursive compilation
				err := c.compileNestedPatternDeclaration(element.Target, extractedReg, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(extractedReg)
					return err
				}
			}
		}

		// Clean up temporary registers
		c.regAlloc.Free(extractedReg)
	}

	return nil
}

// compileObjectDestructuringDeclarationWithValueReg compiles object destructuring declarations using an existing value register
func (c *Compiler) compileObjectDestructuringDeclarationWithValueReg(node *parser.ObjectDestructuringDeclaration, valueReg Register, line int) errors.PaseratiError {
	// ECMAScript compliance: Throw TypeError if destructuring null or undefined
	// This is required at runtime even if type checker catches it at compile time
	c.emitDestructuringNullCheck(valueReg, line)

	// Reuse existing object destructuring logic but skip RHS compilation
	// For each property, compile: define target = valueReg.propertyName
	for _, prop := range node.Properties {
		if prop.Key == nil || prop.Target == nil {
			continue // Skip malformed properties
		}

		// Allocate register for extracted value
		extractedReg := c.regAlloc.Alloc()

		// Handle property access (identifier, number, or computed)
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			// Check if the property name is numeric (for array index access)
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
				var indexNum float64
				_, _ = fmt.Sscanf(keyIdent.Value, "%f", &indexNum)
				indexConstIdx := c.chunk.AddConstant(vm.Number(indexNum))
				indexReg := c.regAlloc.Alloc()
				c.emitLoadConstant(indexReg, indexConstIdx, line)
				c.emitOpCode(vm.OpGetIndex, line)
				c.emitByte(byte(extractedReg))
				c.emitByte(byte(valueReg))
				c.emitByte(byte(indexReg))
				c.regAlloc.Free(indexReg)
			} else {
				// Use OpGetProp for regular string properties
				propNameIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
				c.emitOpCode(vm.OpGetProp, line)
				c.emitByte(byte(extractedReg)) // destination register
				c.emitByte(byte(valueReg))     // object register
				c.emitUint16(propNameIdx)      // property name constant index
			}
		} else if keyNum, ok := prop.Key.(*parser.NumberLiteral); ok {
			// Numeric key - use OpGetIndex for array element access
			indexConstIdx := c.chunk.AddConstant(vm.Number(keyNum.Value))
			indexReg := c.regAlloc.Alloc()
			c.emitLoadConstant(indexReg, indexConstIdx, line)
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(extractedReg))
			c.emitByte(byte(valueReg))
			c.emitByte(byte(indexReg))
			c.regAlloc.Free(indexReg)
		} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
			keyReg := c.regAlloc.Alloc()
			_, err := c.compileNode(computed.Expr, keyReg)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				c.regAlloc.Free(keyReg)
				return err
			}
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(extractedReg))
			c.emitByte(byte(valueReg))
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
					c.regAlloc.Free(extractedReg)
					return err
				}

				// Get the target identifier for conditional assignment
				targetIdent := &parser.Identifier{
					Token: ident.Token,
					Value: ident.Value,
				}

				// Use conditional assignment: target = extractedReg !== undefined ? extractedReg : defaultExpr
				err = c.compileConditionalAssignment(targetIdent, extractedReg, prop.Default, line)
				if err != nil {
					c.regAlloc.Free(extractedReg)
					return err
				}
			} else {
				// Define variable with extracted value
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, extractedReg, line)
				if err != nil {
					c.regAlloc.Free(extractedReg)
					return err
				}
			}
		} else {
			// Nested pattern target (ArrayLiteral or ObjectLiteral)
			if prop.Default != nil {
				// Handle conditional assignment for nested patterns
				err := c.compileConditionalAssignmentForDeclaration(prop.Target, extractedReg, prop.Default, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(extractedReg)
					return err
				}
			} else {
				// Direct nested pattern assignment using recursive compilation
				err := c.compileNestedPatternDeclaration(prop.Target, extractedReg, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(extractedReg)
					return err
				}
			}
		}

		// Clean up temporary register
		c.regAlloc.Free(extractedReg)
	}

	// Handle rest property if present
	if node.RestProperty != nil {
		if ident, ok := node.RestProperty.Target.(*parser.Identifier); ok {
			// Create rest object with remaining properties
			err := c.compileObjectRestDeclaration(valueReg, node.Properties, ident.Value, node.IsConst, line)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// compileConditionalAssignmentForDeclaration handles conditional assignment for nested patterns in declarations
func (c *Compiler) compileConditionalAssignmentForDeclaration(target parser.Expression, valueReg Register, defaultExpr parser.Expression, isConst bool, line int) errors.PaseratiError {
	// This implements: target = (valueReg !== undefined) ? valueReg : defaultExpr
	// IMPORTANT: We must only declare variables ONCE, not in both branches!
	// Solution: Select the value at runtime, store in a single register, then declare once

	// Allocate a result register that will hold either valueReg or defaultReg
	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)

	// 1. Conditional jump: if undefined, jump to default value evaluation
	jumpToDefault := c.emitPlaceholderJump(vm.OpJumpIfUndefined, valueReg, line)

	// 2. Path 1: Value is not undefined, copy it to resultReg
	c.emitMove(resultReg, valueReg, line)

	// Jump past the default evaluation
	jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)

	// 3. Path 2: Value is undefined, evaluate default into resultReg
	c.patchJump(jumpToDefault)

	_, err := c.compileNode(defaultExpr, resultReg)
	if err != nil {
		c.patchJump(jumpPastDefault)
		return err
	}

	// 4. Patch the jump past default
	c.patchJump(jumpPastDefault)

	// 5. Now resultReg contains the correct value (either from valueReg or default)
	// Declare variables once using resultReg
	err = c.compileNestedPatternDeclaration(target, resultReg, isConst, line)
	if err != nil {
		return err
	}

	return nil
}

// compileNestedArrayParameterPattern handles nested array parameter patterns from function parameters
func (c *Compiler) compileNestedArrayParameterPattern(pattern *parser.ArrayParameterPattern, valueReg Register, isConst bool, line int) errors.PaseratiError {
	// ArrayParameterPattern already has Elements as []*DestructuringElement
	// Create a declaration and compile it
	declaration := &parser.ArrayDestructuringDeclaration{
		Token:    pattern.Token,
		IsConst:  isConst,
		Elements: pattern.Elements,
		Value:    nil, // We already have the value in valueReg
	}

	return c.compileArrayDestructuringIteratorPath(declaration, valueReg, line)
}

// compileNestedObjectParameterPattern handles nested object parameter patterns from function parameters
func (c *Compiler) compileNestedObjectParameterPattern(pattern *parser.ObjectParameterPattern, valueReg Register, isConst bool, line int) errors.PaseratiError {
	// ObjectParameterPattern already has Properties as []*DestructuringProperty
	// Create a declaration and compile it
	declaration := &parser.ObjectDestructuringDeclaration{
		Token:        pattern.Token,
		IsConst:      isConst,
		Properties:   pattern.Properties,
		RestProperty: pattern.RestProperty,
		Value:        nil, // We already have the value in valueReg
	}

	return c.compileObjectDestructuringDeclarationWithValueReg(declaration, valueReg, line)
}

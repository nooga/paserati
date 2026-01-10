package compiler

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// compileWithStatement compiles a with statement
func (c *Compiler) compileWithStatement(node *parser.WithStatement, hint Register) (Register, errors.PaseratiError) {
	if node == nil {
		return BadRegister, NewCompileError(node, "nil WithStatement node")
	}

	// line := node.Token.Line // Currently unused but may be needed later

	// Allocate a register for the with object
	objectReg := c.regAlloc.Alloc()

	// Compile the with expression to get the object
	compiledReg, err := c.compileNode(node.Expression, objectReg)
	if err != nil {
		c.regAlloc.Free(objectReg)
		return BadRegister, err
	}

	// If compilation used a different register, we need to move the value
	if compiledReg != objectReg {
		// For now, just use the register that was actually used
		c.regAlloc.Free(objectReg)
		objectReg = compiledReg
	}
	// Don't free objectReg yet - it's needed for property access in the body

	// Extract properties from the type checker's analysis
	var properties map[string]bool
	if node.Expression != nil {
		exprType := node.Expression.GetComputedType()
		properties = c.extractPropertiesFromType(exprType)
	} else {
		properties = make(map[string]bool)
	}

	// Push with object info to symbol table (compile-time tracking)
	c.currentSymbolTable.PushWithObject(objectReg, properties)

	// Emit runtime opcode to push with object onto VM stack
	c.emitPushWithObject(objectReg, node.Token.Line)

	// Increment with block depth (this will be inherited by nested functions)
	c.withBlockDepth++

	// Compile the body with the with object in scope
	var bodyResult Register = BadRegister
	if node.Body != nil {
		bodyResult, err = c.compileNode(node.Body, hint)
		if err != nil {
			// Clean up before returning error
			c.withBlockDepth--
			c.emitPopWithObject(node.Token.Line)
			c.currentSymbolTable.PopWithObject()
			c.regAlloc.Free(objectReg)
			return BadRegister, err
		}
	}

	// Decrement with block depth
	c.withBlockDepth--

	// Emit runtime opcode to pop with object from VM stack
	c.emitPopWithObject(node.Token.Line)

	// Pop the with object when done (compile-time cleanup)
	c.currentSymbolTable.PopWithObject()

	// Now we can free the object register
	c.regAlloc.Free(objectReg)

	return bodyResult, nil
}

// extractPropertiesFromType extracts known property names from a type for compiler use
func (c *Compiler) extractPropertiesFromType(typ types.Type) map[string]bool {
	properties := make(map[string]bool)

	if typ == nil {
		return properties
	}

	switch t := typ.(type) {
	case *types.ObjectType:
		// Mark all properties as known
		for propName := range t.Properties {
			properties[propName] = true
		}
	case *types.Primitive:
		// For 'any' type or other primitives, we can't know properties at compile time
		// Return empty map - this will cause ResolveWithProperty to assume all properties exist
	default:
		// For other types, no known properties
	}

	return properties
}

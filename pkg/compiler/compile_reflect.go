package compiler

import (
	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// compileReflectCall compiles a Paserati.reflect<T>() intrinsic call
// It emits bytecode that creates a type descriptor object at runtime
func (c *Compiler) compileReflectCall(node *parser.CallExpression, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// Get the resolved type from the AST node (set by the checker)
	resolvedType := node.ResolvedReflectType
	if resolvedType == nil {
		c.addError(node, "Paserati.reflect<T>() type was not resolved by the checker")
		return BadRegister, nil
	}

	debugPrintf("// [Compiler] Compiling Paserati.reflect<T>() for type: %s\n", resolvedType.String())

	// Emit bytecode to create the type descriptor object
	return c.emitTypeDescriptor(resolvedType, hint, line)
}

// emitTypeDescriptor emits bytecode to create a type descriptor object for the given type
func (c *Compiler) emitTypeDescriptor(t types.Type, hint Register, line int) (Register, errors.PaseratiError) {
	if t == nil {
		c.emitLoadUndefined(hint, line)
		return hint, nil
	}

	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Create an empty object in hint register
	c.emitMakeEmptyObject(hint, line)

	switch typ := t.(type) {
	case *types.Primitive:
		c.emitSetProperty(hint, "kind", vm.NewString("primitive"), line)
		c.emitSetProperty(hint, "name", vm.NewString(typ.String()), line)

	case *types.LiteralType:
		c.emitSetProperty(hint, "kind", vm.NewString("literal"), line)
		c.emitSetPropertyValue(hint, "value", typ.Value, line, &tempRegs)
		// Infer base type from the value type
		baseType := "unknown"
		switch typ.Value.Type() {
		case vm.TypeString:
			baseType = "string"
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			baseType = "number"
		case vm.TypeBoolean:
			baseType = "boolean"
		case vm.TypeBigInt:
			baseType = "bigint"
		}
		c.emitSetProperty(hint, "baseType", vm.NewString(baseType), line)

	case *types.ObjectType:
		c.emitSetProperty(hint, "kind", vm.NewString("object"), line)

		// Add properties
		if len(typ.Properties) > 0 {
			propsReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, propsReg)
			c.emitMakeEmptyObject(propsReg, line)

			for name, propType := range typ.Properties {
				// Create property descriptor
				propDescReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, propDescReg)
				c.emitMakeEmptyObject(propDescReg, line)

				// Add type field
				typeReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, typeReg)
				c.emitTypeDescriptor(propType, typeReg, line)
				c.emitSetPropertyFromReg(propDescReg, "type", typeReg, line)

				if typ.OptionalProperties[name] {
					c.emitSetProperty(propDescReg, "optional", vm.True, line)
				}
				if typ.ReadOnlyProperties[name] {
					c.emitSetProperty(propDescReg, "readonly", vm.True, line)
				}

				c.emitSetPropertyFromReg(propsReg, name, propDescReg, line)
			}

			c.emitSetPropertyFromReg(hint, "properties", propsReg, line)
		}

		// Add call signatures if callable
		if len(typ.CallSignatures) > 0 {
			sigsReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, sigsReg)
			c.emitMakeEmptyArray(sigsReg, line)

			for _, sig := range typ.CallSignatures {
				sigReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, sigReg)
				c.emitSignatureDescriptor(sig, sigReg, line, &tempRegs)
				c.emitArrayPush(sigsReg, sigReg, line)
			}

			c.emitSetPropertyFromReg(hint, "callSignatures", sigsReg, line)
		}

	case *types.ArrayType:
		c.emitSetProperty(hint, "kind", vm.NewString("array"), line)
		elemReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, elemReg)
		c.emitTypeDescriptor(typ.ElementType, elemReg, line)
		c.emitSetPropertyFromReg(hint, "elementType", elemReg, line)

	case *types.TupleType:
		c.emitSetProperty(hint, "kind", vm.NewString("tuple"), line)
		elemTypesReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, elemTypesReg)
		c.emitMakeEmptyArray(elemTypesReg, line)

		for _, elemType := range typ.ElementTypes {
			elemReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, elemReg)
			c.emitTypeDescriptor(elemType, elemReg, line)
			c.emitArrayPush(elemTypesReg, elemReg, line)
		}
		c.emitSetPropertyFromReg(hint, "elementTypes", elemTypesReg, line)

	case *types.UnionType:
		c.emitSetProperty(hint, "kind", vm.NewString("union"), line)
		typesReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, typesReg)
		c.emitMakeEmptyArray(typesReg, line)

		for _, memberType := range typ.Types {
			memberReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, memberReg)
			c.emitTypeDescriptor(memberType, memberReg, line)
			c.emitArrayPush(typesReg, memberReg, line)
		}
		c.emitSetPropertyFromReg(hint, "types", typesReg, line)

	case *types.IntersectionType:
		c.emitSetProperty(hint, "kind", vm.NewString("intersection"), line)
		typesReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, typesReg)
		c.emitMakeEmptyArray(typesReg, line)

		for _, memberType := range typ.Types {
			memberReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, memberReg)
			c.emitTypeDescriptor(memberType, memberReg, line)
			c.emitArrayPush(typesReg, memberReg, line)
		}
		c.emitSetPropertyFromReg(hint, "types", typesReg, line)

	case *types.GenericType:
		c.emitSetProperty(hint, "kind", vm.NewString("generic"), line)
		c.emitSetProperty(hint, "name", vm.NewString(typ.Name), line)

		// Add type parameters
		paramsReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, paramsReg)
		c.emitMakeEmptyArray(paramsReg, line)

		for _, param := range typ.TypeParameters {
			paramReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, paramReg)
			c.emitMakeEmptyObject(paramReg, line)
			c.emitSetProperty(paramReg, "name", vm.NewString(param.Name), line)

			if param.Constraint != nil {
				constraintReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, constraintReg)
				c.emitTypeDescriptor(param.Constraint, constraintReg, line)
				c.emitSetPropertyFromReg(paramReg, "constraint", constraintReg, line)
			}
			if param.Default != nil {
				defaultReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, defaultReg)
				c.emitTypeDescriptor(param.Default, defaultReg, line)
				c.emitSetPropertyFromReg(paramReg, "default", defaultReg, line)
			}

			c.emitArrayPush(paramsReg, paramReg, line)
		}
		c.emitSetPropertyFromReg(hint, "typeParameters", paramsReg, line)

		// Add body
		bodyReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, bodyReg)
		c.emitTypeDescriptor(typ.Body, bodyReg, line)
		c.emitSetPropertyFromReg(hint, "body", bodyReg, line)

	case *types.InstantiatedType:
		c.emitSetProperty(hint, "kind", vm.NewString("instantiated"), line)
		if typ.Generic != nil {
			c.emitSetProperty(hint, "genericName", vm.NewString(typ.Generic.Name), line)
		}

		// Add type arguments
		argsReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, argsReg)
		c.emitMakeEmptyArray(argsReg, line)

		for _, arg := range typ.TypeArguments {
			argReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, argReg)
			c.emitTypeDescriptor(arg, argReg, line)
			c.emitArrayPush(argsReg, argReg, line)
		}
		c.emitSetPropertyFromReg(hint, "typeArguments", argsReg, line)

		// Add resolved type
		resolvedReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, resolvedReg)
		c.emitTypeDescriptor(typ.Substitute(), resolvedReg, line)
		c.emitSetPropertyFromReg(hint, "resolved", resolvedReg, line)

	case *types.TypeParameterType:
		c.emitSetProperty(hint, "kind", vm.NewString("typeParameter"), line)
		if typ.Parameter != nil {
			c.emitSetProperty(hint, "name", vm.NewString(typ.Parameter.Name), line)
			if typ.Parameter.Constraint != nil {
				constraintReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, constraintReg)
				c.emitTypeDescriptor(typ.Parameter.Constraint, constraintReg, line)
				c.emitSetPropertyFromReg(hint, "constraint", constraintReg, line)
			}
		}

	case *types.ClassType:
		c.emitSetProperty(hint, "kind", vm.NewString("class"), line)
		c.emitSetProperty(hint, "name", vm.NewString(typ.Name), line)

		instanceReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, instanceReg)
		c.emitTypeDescriptor(typ.InstanceType, instanceReg, line)
		c.emitSetPropertyFromReg(hint, "instanceType", instanceReg, line)

		staticReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, staticReg)
		c.emitTypeDescriptor(typ.StaticType, staticReg, line)
		c.emitSetPropertyFromReg(hint, "staticType", staticReg, line)

	default:
		// Fallback for any other types
		c.emitSetProperty(hint, "kind", vm.NewString("unknown"), line)
		c.emitSetProperty(hint, "name", vm.NewString(t.String()), line)
	}

	return hint, nil
}

// emitSignatureDescriptor emits bytecode to create a signature descriptor
func (c *Compiler) emitSignatureDescriptor(sig *types.Signature, hint Register, line int, tempRegs *[]Register) {
	c.emitMakeEmptyObject(hint, line)

	// Add parameter types
	paramsReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, paramsReg)
	c.emitMakeEmptyArray(paramsReg, line)

	for i, paramType := range sig.ParameterTypes {
		paramReg := c.regAlloc.Alloc()
		*tempRegs = append(*tempRegs, paramReg)
		c.emitMakeEmptyObject(paramReg, line)

		typeReg := c.regAlloc.Alloc()
		*tempRegs = append(*tempRegs, typeReg)
		c.emitTypeDescriptor(paramType, typeReg, line)
		c.emitSetPropertyFromReg(paramReg, "type", typeReg, line)

		if i < len(sig.OptionalParams) && sig.OptionalParams[i] {
			c.emitSetProperty(paramReg, "optional", vm.True, line)
		}

		c.emitArrayPush(paramsReg, paramReg, line)
	}
	c.emitSetPropertyFromReg(hint, "parameters", paramsReg, line)

	// Add return type
	if sig.ReturnType != nil {
		returnReg := c.regAlloc.Alloc()
		*tempRegs = append(*tempRegs, returnReg)
		c.emitTypeDescriptor(sig.ReturnType, returnReg, line)
		c.emitSetPropertyFromReg(hint, "returnType", returnReg, line)
	}

	// Add rest parameter type
	if sig.RestParameterType != nil {
		restReg := c.regAlloc.Alloc()
		*tempRegs = append(*tempRegs, restReg)
		c.emitTypeDescriptor(sig.RestParameterType, restReg, line)
		c.emitSetPropertyFromReg(hint, "restParameter", restReg, line)
	}

	if sig.IsVariadic {
		c.emitSetProperty(hint, "isVariadic", vm.True, line)
	}
}

// Helper methods for emitting property sets

// emitSetProperty sets a property to a constant value
func (c *Compiler) emitSetProperty(objReg Register, propName string, value vm.Value, line int) {
	valueReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(valueReg)

	constIdx := c.chunk.AddConstant(value)
	c.emitLoadConstant(valueReg, constIdx, line)

	propIdx := c.chunk.AddConstant(vm.NewString(propName))
	c.emitOpCode(vm.OpSetProp, line)
	c.emitByte(byte(objReg))
	c.emitByte(byte(valueReg))
	c.emitUint16(propIdx)
}

// emitSetPropertyValue sets a property from a vm.Value (may need temporary register)
func (c *Compiler) emitSetPropertyValue(objReg Register, propName string, value vm.Value, line int, tempRegs *[]Register) {
	valueReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, valueReg)

	constIdx := c.chunk.AddConstant(value)
	c.emitLoadConstant(valueReg, constIdx, line)

	propIdx := c.chunk.AddConstant(vm.NewString(propName))
	c.emitOpCode(vm.OpSetProp, line)
	c.emitByte(byte(objReg))
	c.emitByte(byte(valueReg))
	c.emitUint16(propIdx)
}

// emitSetPropertyFromReg sets a property from a register value
func (c *Compiler) emitSetPropertyFromReg(objReg Register, propName string, valueReg Register, line int) {
	propIdx := c.chunk.AddConstant(vm.NewString(propName))
	c.emitOpCode(vm.OpSetProp, line)
	c.emitByte(byte(objReg))
	c.emitByte(byte(valueReg))
	c.emitUint16(propIdx)
}

// emitMakeEmptyArray emits bytecode to create an empty array
func (c *Compiler) emitMakeEmptyArray(dest Register, line int) {
	// OpMakeArray format: dest, startReg, count
	c.emitOpCode(vm.OpMakeArray, line)
	c.emitByte(byte(dest))
	c.emitByte(0) // startReg (unused for empty array)
	c.emitByte(0) // count (0 elements)
}

// emitArrayPush emits bytecode to push a value onto an array
func (c *Compiler) emitArrayPush(arrayReg Register, valueReg Register, line int) {
	// For OpCallMethod, arguments must be in consecutive registers starting at funcReg+1
	pushMethodReg := c.regAlloc.Alloc()
	pushArgReg := c.regAlloc.Alloc() // This will be pushMethodReg+1

	pushIdx := c.chunk.AddConstant(vm.NewString("push"))
	c.emitGetProp(pushMethodReg, arrayReg, pushIdx, line)

	// Move value to argument position (pushMethodReg+1)
	c.emitMove(pushArgReg, valueReg, line)

	// Call push method with 1 argument - use arrayReg as 'this'
	resultReg := c.regAlloc.Alloc()
	c.emitCallMethod(resultReg, pushMethodReg, arrayReg, 1, line)

	// Free immediately
	c.regAlloc.Free(resultReg)
	c.regAlloc.Free(pushArgReg)
	c.regAlloc.Free(pushMethodReg)
}

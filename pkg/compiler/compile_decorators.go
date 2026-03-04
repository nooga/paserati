package compiler

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// decoratorInfo holds a pre-evaluated decorator expression and the register it's stored in
type decoratorInfo struct {
	reg    Register
	line   int
	target string // "class", "method", "getter", "setter", "field"
	name   string // property name (for class elements)
}

// preEvaluateDecorators evaluates all decorator expressions at class definition time.
// Per TC39 spec, decorator expressions are evaluated in source order, interleaved with
// computed property keys. This function evaluates class decorators and all member decorators,
// storing results in registers for later application.
func (c *Compiler) preEvaluateDecorators(node *parser.ClassDeclaration) ([]*decoratorInfo, errors.PaseratiError) {
	var decorators []*decoratorInfo

	// Class-level decorators (evaluated first, in source order)
	for _, dec := range node.Decorators {
		info, err := c.evaluateDecoratorExpr(dec, "class", node.Name.Value)
		if err != nil {
			return nil, err
		}
		decorators = append(decorators, info)
	}

	// Member decorators (evaluated in source order)
	for _, method := range node.Body.Methods {
		if method.Kind == "constructor" {
			continue // constructors cannot be decorated
		}
		for _, dec := range method.Decorators {
			kind := "method"
			if method.Kind == "getter" {
				kind = "getter"
			} else if method.Kind == "setter" {
				kind = "setter"
			}
			info, err := c.evaluateDecoratorExpr(dec, kind, c.extractPropertyName(method.Key))
			if err != nil {
				return nil, err
			}
			decorators = append(decorators, info)
		}
	}

	for _, prop := range node.Body.Properties {
		for _, dec := range prop.Decorators {
			info, err := c.evaluateDecoratorExpr(dec, "field", c.extractPropertyName(prop.Key))
			if err != nil {
				return nil, err
			}
			decorators = append(decorators, info)
		}
	}

	return decorators, nil
}

// evaluateDecoratorExpr compiles a single decorator expression and stores the result in a register
func (c *Compiler) evaluateDecoratorExpr(dec *parser.Decorator, target, name string) (*decoratorInfo, errors.PaseratiError) {
	reg := c.regAlloc.Alloc()
	_, err := c.compileNode(dec.Expression, reg)
	if err != nil {
		c.regAlloc.Free(reg)
		return nil, err
	}
	return &decoratorInfo{
		reg:    reg,
		line:   dec.Token.Line,
		target: target,
		name:   name,
	}, nil
}

// freeDecoratorRegs frees all registers used by pre-evaluated decorators
func (c *Compiler) freeDecoratorRegs(decorators []*decoratorInfo) {
	for _, d := range decorators {
		c.regAlloc.Free(d.reg)
	}
}

// emitMakeAddInitializer creates an addInitializer function in destReg that pushes to arrayReg
func (c *Compiler) emitMakeAddInitializer(destReg, arrayReg Register, line int) {
	c.emitOpCode(vm.OpMakeAddInitializer, line)
	c.emitByte(byte(destReg))
	c.emitByte(byte(arrayReg))
}

// emitRunInitializers runs all initializer functions in arrayReg with 'this' = thisReg
func (c *Compiler) emitRunInitializers(arrayReg, thisReg Register, line int) {
	c.emitOpCode(vm.OpRunInitializers, line)
	c.emitByte(byte(arrayReg))
	c.emitByte(byte(thisReg))
}

// applyClassDecorators applies class-level decorators to the constructor.
// Per TC39 spec, class decorators are called bottom-to-top (reverse of source order).
// Each decorator receives (classValue, context) and can return a replacement class.
func (c *Compiler) applyClassDecorators(node *parser.ClassDeclaration, constructorReg Register, decorators []*decoratorInfo) errors.PaseratiError {
	var classDecorators []*decoratorInfo
	for _, d := range decorators {
		if d.target == "class" {
			classDecorators = append(classDecorators, d)
		}
	}

	if len(classDecorators) == 0 {
		return nil
	}

	// Create initializer array for addInitializer callbacks
	initArrayReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(initArrayReg)
	c.emitMakeEmptyArray(initArrayReg, classDecorators[0].line)

	// Apply in reverse order (bottom-to-top per TC39 spec)
	for i := len(classDecorators) - 1; i >= 0; i-- {
		dec := classDecorators[i]

		// Create context object: { kind: "class", name: className, addInitializer }
		contextReg := c.regAlloc.Alloc()
		c.createDecoratorContext(contextReg, "class", node.Name.Value, false, false, initArrayReg, dec.line)

		// Call: result = decorator(classConstructor, context)
		callRegs := c.regAlloc.AllocContiguous(3)
		c.emitMove(callRegs, dec.reg, dec.line)
		c.emitMove(callRegs+1, constructorReg, dec.line)
		c.emitMove(callRegs+2, contextReg, dec.line)

		resultReg := c.regAlloc.Alloc()
		c.emitCall(resultReg, callRegs, 2, dec.line)

		// If decorator returned a value (not undefined), replace the class
		c.emitCheckAndReplace(constructorReg, resultReg, dec.line)

		c.regAlloc.Free(resultReg)
		c.regAlloc.Free(callRegs + 2)
		c.regAlloc.Free(callRegs + 1)
		c.regAlloc.Free(callRegs)
		c.regAlloc.Free(contextReg)
	}

	// Run class decorator initializers with 'this' = constructor
	c.emitRunInitializers(initArrayReg, constructorReg, classDecorators[0].line)

	return nil
}

// applyMethodDecorators applies all decorators to a method in reverse order.
// The method has already been compiled into methodReg.
func (c *Compiler) applyMethodDecorators(decs []*decoratorInfo, methodReg Register, method *parser.MethodDefinition) errors.PaseratiError {
	if len(decs) == 0 {
		return nil
	}

	propName := c.extractPropertyName(method.Key)
	isPrivate := method.IsPrivate || (len(propName) > 0 && propName[0] == '#')

	// Create initializer array for addInitializer callbacks
	initArrayReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(initArrayReg)
	c.emitMakeEmptyArray(initArrayReg, decs[0].line)

	// Apply in reverse order (bottom-to-top per TC39 spec)
	for i := len(decs) - 1; i >= 0; i-- {
		dec := decs[i]

		contextReg := c.regAlloc.Alloc()
		c.createDecoratorContext(contextReg, dec.target, propName, method.IsStatic, isPrivate, initArrayReg, dec.line)

		callRegs := c.regAlloc.AllocContiguous(3)
		c.emitMove(callRegs, dec.reg, dec.line)
		c.emitMove(callRegs+1, methodReg, dec.line)
		c.emitMove(callRegs+2, contextReg, dec.line)

		resultReg := c.regAlloc.Alloc()
		c.emitCall(resultReg, callRegs, 2, dec.line)

		c.emitCheckAndReplace(methodReg, resultReg, dec.line)

		c.regAlloc.Free(resultReg)
		c.regAlloc.Free(callRegs + 2)
		c.regAlloc.Free(callRegs + 1)
		c.regAlloc.Free(callRegs)
		c.regAlloc.Free(contextReg)
	}

	// For method decorators, initializers are stored for later execution.
	// Static member initializers run after class definition.
	// Instance member initializers run in the constructor.
	// For now, we run them immediately (with undefined as this since the class isn't fully set up yet).
	// TODO: properly defer instance initializers to constructor time
	undefinedReg := c.regAlloc.Alloc()
	c.emitLoadUndefined(undefinedReg, decs[0].line)
	c.emitRunInitializers(initArrayReg, undefinedReg, decs[0].line)
	c.regAlloc.Free(undefinedReg)

	return nil
}

// createDecoratorContext emits bytecode to create a decorator context object.
// Context structure: { kind, name, static, private, addInitializer }
func (c *Compiler) createDecoratorContext(destReg Register, kind, name string, isStatic, isPrivate bool, initArrayReg Register, line int) {
	c.emitMakeEmptyObject(destReg, line)

	// Set kind
	kindNameIdx := c.chunk.AddConstant(vm.String("kind"))
	kindValIdx := c.chunk.AddConstant(vm.String(kind))
	kindValReg := c.regAlloc.Alloc()
	c.emitLoadConstant(kindValReg, kindValIdx, line)
	c.emitSetProp(destReg, kindValReg, kindNameIdx, line)
	c.regAlloc.Free(kindValReg)

	// Set name
	nameNameIdx := c.chunk.AddConstant(vm.String("name"))
	nameValIdx := c.chunk.AddConstant(vm.String(name))
	nameValReg := c.regAlloc.Alloc()
	c.emitLoadConstant(nameValReg, nameValIdx, line)
	c.emitSetProp(destReg, nameValReg, nameNameIdx, line)
	c.regAlloc.Free(nameValReg)

	// Set static and private (for class elements, not for class decorator)
	if kind != "class" {
		staticNameIdx := c.chunk.AddConstant(vm.String("static"))
		staticValReg := c.regAlloc.Alloc()
		if isStatic {
			c.emitLoadTrue(staticValReg, line)
		} else {
			c.emitLoadFalse(staticValReg, line)
		}
		c.emitSetProp(destReg, staticValReg, staticNameIdx, line)
		c.regAlloc.Free(staticValReg)

		privateNameIdx := c.chunk.AddConstant(vm.String("private"))
		privateValReg := c.regAlloc.Alloc()
		if isPrivate {
			c.emitLoadTrue(privateValReg, line)
		} else {
			c.emitLoadFalse(privateValReg, line)
		}
		c.emitSetProp(destReg, privateValReg, privateNameIdx, line)
		c.regAlloc.Free(privateValReg)
	}

	// Set addInitializer function
	addInitNameIdx := c.chunk.AddConstant(vm.String("addInitializer"))
	addInitReg := c.regAlloc.Alloc()
	c.emitMakeAddInitializer(addInitReg, initArrayReg, line)
	c.emitSetProp(destReg, addInitReg, addInitNameIdx, line)
	c.regAlloc.Free(addInitReg)
}

// emitCheckAndReplace emits bytecode that checks if resultReg is not undefined,
// and if so, moves resultReg into destReg. Implements the TC39 decorator
// semantics where returning undefined means "keep the original".
func (c *Compiler) emitCheckAndReplace(destReg, resultReg Register, line int) {
	// If result is undefined, skip past the move
	jumpPos := c.emitPlaceholderJump(vm.OpJumpIfUndefined, resultReg, line)

	// result !== undefined: replace dest with result
	c.emitMove(destReg, resultReg, line)

	// Patch the jump target to here
	c.patchJump(jumpPos)
}

// getMethodDecorators returns decorator infos matching a specific method
func (c *Compiler) getMethodDecorators(method *parser.MethodDefinition, allDecorators []*decoratorInfo) []*decoratorInfo {
	if len(method.Decorators) == 0 {
		return nil
	}
	propName := c.extractPropertyName(method.Key)
	kind := "method"
	if method.Kind == "getter" {
		kind = "getter"
	} else if method.Kind == "setter" {
		kind = "setter"
	}

	var result []*decoratorInfo
	for _, d := range allDecorators {
		if d.target == kind && d.name == propName {
			result = append(result, d)
		}
	}
	return result
}

// hasDecorators checks if a class declaration or any of its members have decorators
func hasDecorators(node *parser.ClassDeclaration) bool {
	if len(node.Decorators) > 0 {
		return true
	}
	for _, method := range node.Body.Methods {
		if len(method.Decorators) > 0 {
			return true
		}
	}
	for _, prop := range node.Body.Properties {
		if len(prop.Decorators) > 0 {
			return true
		}
	}
	return false
}

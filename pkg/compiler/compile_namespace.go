package compiler

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// compileNamespaceDeclaration compiles a TypeScript namespace declaration to
// bytecode that mirrors tsc's IIFE pattern, but inline (no synthetic IIFE
// function). The runtime object lives:
//   - For a top-level namespace `N`: in the global slot for `N`.
//   - For a nested namespace `Outer.Inner`: as `Outer.Inner` on the parent.
//
// Inside the body:
//   - Exported `let/var/const/function/class/enum`: stored into `<ns>.x` and
//     bound in the body's symbol table as a NamespaceProperty so internal
//     references compile to property reads/writes on the namespace object.
//     This keeps internal mutation in sync with external observation.
//   - Non-exported declarations: ordinary locals scoped to the body.
//   - Interfaces / type aliases: no codegen.
//   - Nested namespaces: recursive compilation; if exported, bound as
//     NamespaceProperty in the parent's body scope.
func (c *Compiler) compileNamespaceDeclaration(node *parser.NamespaceDeclaration, hint Register) (Register, errors.PaseratiError) {
	if node == nil || node.Name == nil {
		return hint, nil
	}
	if node.Declare {
		// Ambient namespace: type-only, no runtime emission.
		return hint, nil
	}
	line := node.Token.Line
	nsName := node.Name.Value

	// Build the access expression for THIS namespace's runtime object.
	var accessExpr parser.Expression
	if c.currentNamespaceAccess == nil {
		// Top-level namespace: bind to a global slot, initialize on first
		// declaration, merge on subsequent ones.
		globalIdx := c.GetOrAssignGlobalIndex(nsName)
		_, _, alreadyDefined := c.currentSymbolTable.Resolve(nsName)
		if !alreadyDefined {
			c.currentSymbolTable.DefineGlobal(nsName, globalIdx)
		}

		tempReg := c.regAlloc.Alloc()
		if !alreadyDefined {
			// First occurrence: unconditionally initialize to {}. We can't read
			// the global first because OpGetGlobal throws ReferenceError for an
			// unbound name.
			c.emitMakeEmptyObject(tempReg, line)
			c.emitSetGlobal(globalIdx, tempReg, line)
		} else {
			// Merging an existing namespace declaration: keep the existing
			// object if truthy, otherwise replace with {}.
			c.emitGetGlobal(tempReg, globalIdx, line)
			jumpIfFalsy := c.emitPlaceholderJump(vm.OpJumpIfFalse, tempReg, line)
			jumpToEnd := c.emitPlaceholderJump(vm.OpJump, 0, line)
			c.patchJump(jumpIfFalsy)
			c.emitMakeEmptyObject(tempReg, line)
			c.emitSetGlobal(globalIdx, tempReg, line)
			c.patchJump(jumpToEnd)
		}
		c.regAlloc.Free(tempReg)

		accessExpr = &parser.Identifier{Token: node.Name.Token, Value: nsName}
	} else {
		// Nested: ensure parent.<nsName> exists.
		parentReg := c.regAlloc.Alloc()
		if _, err := c.compileNode(c.currentNamespaceAccess, parentReg); err != nil {
			c.regAlloc.Free(parentReg)
			return BadRegister, err
		}
		nameIdx := c.chunk.AddConstant(vm.String(nsName))

		propReg := c.regAlloc.Alloc()
		c.emitGetProp(propReg, parentReg, uint16(nameIdx), line)
		jumpIfFalsy := c.emitPlaceholderJump(vm.OpJumpIfFalse, propReg, line)
		jumpToEnd := c.emitPlaceholderJump(vm.OpJump, 0, line)
		c.patchJump(jumpIfFalsy)
		c.emitMakeEmptyObject(propReg, line)
		c.emitSetProp(parentReg, propReg, uint16(nameIdx), line)
		c.patchJump(jumpToEnd)
		c.regAlloc.Free(propReg)
		c.regAlloc.Free(parentReg)

		accessExpr = &parser.MemberExpression{
			Token:    node.Name.Token,
			Object:   c.currentNamespaceAccess,
			Property: &parser.Identifier{Token: node.Name.Token, Value: nsName},
		}
	}

	// If we are inside another namespace's body and THIS namespace is exported,
	// the enclosing predeclare loop already bound `nsName` as NamespaceProperty
	// in the parent's symbol table. Nothing extra to do here.

	// Push namespace context: open a fresh block scope for the body.
	prevAccess := c.currentNamespaceAccess
	c.currentNamespaceAccess = accessExpr
	prevTable := c.currentSymbolTable
	c.currentSymbolTable = NewEnclosedSymbolTable(prevTable)
	defer func() {
		c.currentSymbolTable = prevTable
		c.currentNamespaceAccess = prevAccess
	}()

	if node.Body == nil {
		return hint, nil
	}

	// Predeclare exports as NamespaceProperty bindings BEFORE compiling any
	// body statement, so forward references inside (e.g. function bodies that
	// reference a later-declared exported variable) resolve correctly.
	c.predeclareNamespaceExports(node.Body, accessExpr)

	// Hoist function declarations: emit their property assignments before any
	// other body statements so forward references like `const r = f(3)` see f.
	for _, stmt := range node.Body.Statements {
		if isNamespaceFunctionDeclaration(stmt) {
			if err := c.compileNamespaceBodyStatement(stmt, accessExpr); err != nil {
				return BadRegister, err
			}
		}
	}
	// Compile remaining body statements.
	for _, stmt := range node.Body.Statements {
		if stmt == nil || isNamespaceFunctionDeclaration(stmt) {
			continue
		}
		if err := c.compileNamespaceBodyStatement(stmt, accessExpr); err != nil {
			return BadRegister, err
		}
	}

	return hint, nil
}

// isNamespaceFunctionDeclaration reports whether stmt is a (possibly exported)
// function declaration inside a namespace body. Such declarations are hoisted
// to the top of the body to match TypeScript's IIFE-emission semantics.
func isNamespaceFunctionDeclaration(stmt parser.Statement) bool {
	inner := stmt
	if exp, ok := stmt.(*parser.ExportNamedDeclaration); ok && exp.Declaration != nil {
		inner = exp.Declaration
	}
	exprStmt, ok := inner.(*parser.ExpressionStatement)
	if !ok {
		return false
	}
	fn, ok := exprStmt.Expression.(*parser.FunctionLiteral)
	return ok && fn.Name != nil
}

// predeclareNamespaceExports walks the body and binds every exported
// value-name as a NamespaceProperty symbol in the current scope. This must run
// before the compile pass so that references inside function bodies resolve.
func (c *Compiler) predeclareNamespaceExports(body *parser.BlockStatement, accessExpr parser.Expression) {
	for _, stmt := range body.Statements {
		exp, ok := stmt.(*parser.ExportNamedDeclaration)
		if !ok || exp.Declaration == nil {
			continue
		}
		switch d := exp.Declaration.(type) {
		case *parser.LetStatement:
			for _, decl := range d.Declarations {
				if decl != nil && decl.Name != nil {
					c.currentSymbolTable.DefineNamespaceProperty(decl.Name.Value, accessExpr)
				}
			}
		case *parser.ConstStatement:
			for _, decl := range d.Declarations {
				if decl != nil && decl.Name != nil {
					c.currentSymbolTable.DefineNamespaceProperty(decl.Name.Value, accessExpr)
				}
			}
		case *parser.VarStatement:
			for _, decl := range d.Declarations {
				if decl != nil && decl.Name != nil {
					c.currentSymbolTable.DefineNamespaceProperty(decl.Name.Value, accessExpr)
				}
			}
		case *parser.ClassDeclaration:
			if d.Name != nil {
				c.currentSymbolTable.DefineNamespaceProperty(d.Name.Value, accessExpr)
			}
		case *parser.NamespaceDeclaration:
			if d.Name != nil {
				c.currentSymbolTable.DefineNamespaceProperty(d.Name.Value, accessExpr)
			}
		case *parser.ExpressionStatement:
			if fn, ok := d.Expression.(*parser.FunctionLiteral); ok && fn.Name != nil {
				c.currentSymbolTable.DefineNamespaceProperty(fn.Name.Value, accessExpr)
			} else if en, ok := d.Expression.(*parser.EnumDeclaration); ok && en.Name != nil {
				c.currentSymbolTable.DefineNamespaceProperty(en.Name.Value, accessExpr)
			}
		}
	}
}

// compileNamespaceBodyStatement compiles a single statement inside a namespace
// body. Exported declarations have their RHS compiled and then assigned to the
// namespace object via OpSetProp. Non-exported declarations are compiled in the
// usual way as locals.
func (c *Compiler) compileNamespaceBodyStatement(stmt parser.Statement, accessExpr parser.Expression) errors.PaseratiError {
	exp, isExport := stmt.(*parser.ExportNamedDeclaration)
	if !isExport {
		// Non-exported: compile normally. Skip type-only constructs (interfaces,
		// type aliases, declare statements) since they have no runtime effect.
		switch stmt.(type) {
		case *parser.InterfaceDeclaration, *parser.TypeAliasStatement:
			return nil
		}
		hint := c.regAlloc.Alloc()
		_, err := c.compileNode(stmt, hint)
		c.regAlloc.Free(hint)
		return err
	}

	// Exported. Process by declaration kind.
	if exp.Declaration == nil {
		return nil
	}
	switch d := exp.Declaration.(type) {
	case *parser.InterfaceDeclaration, *parser.TypeAliasStatement:
		// Type-only: no codegen.
		return nil

	case *parser.LetStatement:
		return c.compileNamespaceVarLikeExports(d.Declarations, accessExpr)
	case *parser.ConstStatement:
		return c.compileNamespaceVarLikeExports(d.Declarations, accessExpr)
	case *parser.VarStatement:
		return c.compileNamespaceVarLikeExports(d.Declarations, accessExpr)

	case *parser.ClassDeclaration:
		return c.compileNamespaceClassExport(d, accessExpr)

	case *parser.NamespaceDeclaration:
		hint := c.regAlloc.Alloc()
		_, err := c.compileNamespaceDeclaration(d, hint)
		c.regAlloc.Free(hint)
		return err

	case *parser.ExpressionStatement:
		if fn, ok := d.Expression.(*parser.FunctionLiteral); ok && fn.Name != nil {
			return c.compileNamespaceFunctionExport(fn, accessExpr)
		}
		if en, ok := d.Expression.(*parser.EnumDeclaration); ok && en.Name != nil {
			return c.compileNamespaceEnumExport(en, accessExpr)
		}
		hint := c.regAlloc.Alloc()
		_, err := c.compileNode(d, hint)
		c.regAlloc.Free(hint)
		return err
	}

	// Fallback: compile the inner declaration normally.
	hint := c.regAlloc.Alloc()
	_, err := c.compileNode(exp.Declaration, hint)
	c.regAlloc.Free(hint)
	return err
}

// compileNamespaceVarLikeExports emits `<ns>.X = initializer` for each declarator.
// Compound declarations like `export const a = 1, b = 2;` produce one assignment
// per declarator.
func (c *Compiler) compileNamespaceVarLikeExports(decls []*parser.VarDeclarator, accessExpr parser.Expression) errors.PaseratiError {
	for _, decl := range decls {
		if decl == nil || decl.Name == nil {
			continue
		}
		assign := &parser.AssignmentExpression{
			Token: decl.Name.Token,
			Operator: "=",
			Left: &parser.MemberExpression{
				Token:    decl.Name.Token,
				Object:   accessExpr,
				Property: &parser.Identifier{Token: decl.Name.Token, Value: decl.Name.Value},
			},
			Value: decl.Value,
		}
		if assign.Value == nil {
			// `export var x;` with no initializer — define as undefined property.
			assign.Value = &parser.UndefinedLiteral{Token: decl.Name.Token}
		}
		hint := c.regAlloc.Alloc()
		_, err := c.compileNode(assign, hint)
		c.regAlloc.Free(hint)
		if err != nil {
			return err
		}
	}
	return nil
}

// compileNamespaceFunctionExport compiles `export function f(...) { body }` as
// `<ns>.f = function f(...) { body }`. The function literal's body is compiled
// in the current scope, so references to other exports inside f's body resolve
// to namespace properties.
func (c *Compiler) compileNamespaceFunctionExport(fn *parser.FunctionLiteral, accessExpr parser.Expression) errors.PaseratiError {
	assign := &parser.AssignmentExpression{
		Token:    fn.Token,
		Operator: "=",
		Left: &parser.MemberExpression{
			Token:    fn.Token,
			Object:   accessExpr,
			Property: &parser.Identifier{Token: fn.Token, Value: fn.Name.Value},
		},
		Value: fn,
	}
	hint := c.regAlloc.Alloc()
	_, err := c.compileNode(assign, hint)
	c.regAlloc.Free(hint)
	return err
}

// compileNamespaceClassExport compiles `export class C { ... }` as
// `<ns>.C = class C { ... }`. We turn the ClassDeclaration into a
// ClassExpression so the class compiles into a value register without
// installing a top-level binding.
func (c *Compiler) compileNamespaceClassExport(cd *parser.ClassDeclaration, accessExpr parser.Expression) errors.PaseratiError {
	classExpr := &parser.ClassExpression{
		Token:          cd.Token,
		Name:           cd.Name,
		TypeParameters: cd.TypeParameters,
		SuperClass:     cd.SuperClass,
		Body:           cd.Body,
		IsAbstract:     cd.IsAbstract,
	}
	assign := &parser.AssignmentExpression{
		Token:    cd.Token,
		Operator: "=",
		Left: &parser.MemberExpression{
			Token:    cd.Token,
			Object:   accessExpr,
			Property: &parser.Identifier{Token: cd.Token, Value: cd.Name.Value},
		},
		Value: classExpr,
	}
	hint := c.regAlloc.Alloc()
	_, err := c.compileNode(assign, hint)
	c.regAlloc.Free(hint)
	return err
}

// compileNamespaceEnumExport compiles `export enum E { ... }` as
// `<ns>.E = E_value`. The enum declaration both defines a global and produces
// the enum object value in `hint`; we then copy that value into the namespace
// property.
func (c *Compiler) compileNamespaceEnumExport(en *parser.EnumDeclaration, accessExpr parser.Expression) errors.PaseratiError {
	enumReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(enumReg)
	if _, err := c.compileNode(en, enumReg); err != nil {
		return err
	}
	nameIdx := c.chunk.AddConstant(vm.String(en.Name.Value))
	objReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(objReg)
	if _, err := c.compileNode(accessExpr, objReg); err != nil {
		return err
	}
	c.emitSetProp(objReg, enumReg, uint16(nameIdx), en.Token.Line)
	return nil
}

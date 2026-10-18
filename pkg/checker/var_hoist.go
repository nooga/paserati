package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// hoistVarNames pre-declares every `var` binding found in stmts (looking
// through nested blocks, loops, try/switch, but not into nested functions) in
// scope. `var` bindings exist from the start of their function (or script), so
// a read that precedes the declaration - `f(i); var i = 1;`, or a use inside a
// block that sits before a later `var` - resolves instead of being reported as
// TS2304. The real declaration, when visited, finds the name already present
// and refines its type.
//
// Names that already exist in scope (parameters, Pass 2 hoisting) are left
// untouched.
func (c *Checker) hoistVarNames(scope *Environment, stmts []parser.Statement) {
	if scope == nil {
		return
	}
	for _, name := range parser.VarDeclaredNames(stmts) {
		if _, found := scope.symbols[name]; found {
			continue
		}
		scope.Define(name, types.Any, false)
	}
}

// hoistFunctionBodyVars hoists the var bindings of a function body block into
// the current function scope.
func (c *Checker) hoistFunctionBodyVars(body parser.Node) {
	block, ok := body.(*parser.BlockStatement)
	if !ok || block == nil {
		return
	}
	c.hoistVarNames(c.env.GetFunctionScope(), block.Statements)
}

// collectVarOnlyDecls returns, for each name a single `var` declarator
// declares and nothing else in program does (no let, const, parameter,
// function or class), that declarator.
func collectVarOnlyDecls(program *parser.Program) map[string]*parser.VarDeclarator {
	decls := map[string]*parser.VarDeclarator{}
	other := map[string]bool{}
	forEachDescendant(program, func(n parser.Node) {
		switch x := n.(type) {
		case *parser.VarStatement:
			for _, d := range x.Declarations {
				if d != nil && d.Name != nil {
					if _, seen := decls[d.Name.Value]; seen {
						other[d.Name.Value] = true
					}
					decls[d.Name.Value] = d
				}
			}
		case *parser.LetStatement:
			for _, d := range x.Declarations {
				if d != nil && d.Name != nil {
					other[d.Name.Value] = true
				}
			}
		case *parser.ConstStatement:
			for _, d := range x.Declarations {
				if d != nil && d.Name != nil {
					other[d.Name.Value] = true
				}
			}
		case *parser.Parameter:
			if x.Name != nil {
				other[x.Name.Value] = true
			}
		case *parser.FunctionLiteral:
			if x.Name != nil {
				other[x.Name.Value] = true
			}
			for _, p := range x.Parameters {
				if p != nil && p.Name != nil {
					other[p.Name.Value] = true
				}
			}
		case *parser.ArrowFunctionLiteral:
			for _, p := range x.Parameters {
				if p != nil && p.Name != nil {
					other[p.Name.Value] = true
				}
			}
		case *parser.ClassDeclaration:
			if x.Name != nil {
				other[x.Name.Value] = true
			}
		}
	})
	for name := range other {
		delete(decls, name)
	}
	return decls
}

package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// checkNamespaceDeclaration handles `namespace X { ... }`, `export namespace X { ... }`,
// and `declare namespace X { ... }` declarations.
//
// Semantics (matching tsc):
//   - The namespace name is bound BOTH in the VALUE env (to the runtime ObjectType
//     describing exported value bindings) AND in the TYPE env (to the
//     NamespaceType wrapper, which also carries TypeMembers).
//   - A second `namespace X` block in the same scope MERGES into the existing
//     NamespaceType.
//   - Inside the body, only `export ...` declarations contribute to the
//     namespace's exports; non-exported declarations are local to the body env.
//   - `declare namespace` is type-only (no runtime effect), but contributes the
//     same bindings.
func (c *Checker) checkNamespaceDeclaration(node *parser.NamespaceDeclaration) {
	if node == nil || node.Name == nil {
		return
	}
	name := node.Name.Value

	// 1. Get-or-create the NamespaceType in the current scope. We look in the
	//    type env: a NamespaceType always lives there.
	var nsType *types.NamespaceType
	if existing, found := c.env.ResolveType(name); found {
		if ns, ok := existing.(*types.NamespaceType); ok {
			nsType = ns
		}
	}
	if nsType == nil {
		nsType = types.NewNamespaceType(name)
		nsType.Declare = node.Declare
		// Bind in type env (for namespace-qualified type names like N.X).
		c.env.DefineTypeAlias(name, nsType)
		// Bind ValueShape in value env so `N.x` member access resolves through
		// ordinary ObjectType property lookup.
		c.env.Define(name, nsType.ValueShape, false)
	}

	// 2. Create an enclosed environment for the body.
	outerEnv := c.env
	bodyEnv := NewEnclosedEnvironment(outerEnv)
	c.env = bodyEnv

	// Within the body, the namespace name itself should still resolve. The
	// outer-scope bindings already cover this via lexical lookup.

	// 3. Walk each body statement and dispatch. We deliberately use a single-
	//    pass walk inside namespaces (good enough for our smoke tests; can be
	//    upgraded to multi-pass if needed later).
	if node.Body != nil {
		// Mirror the top-level checker passes:
		//   Pass A: interfaces, type aliases, classes, nested namespaces (types).
		//   Pass B: hoist function signatures (so interfaces are available for
		//           parameter type resolution).
		//   Pass C: visit remaining body statements.
		c.preprocessNamespaceTypes(node.Body, nsType)
		c.hoistNamespaceFunctions(node.Body, bodyEnv)

		for _, stmt := range node.Body.Statements {
			if stmt == nil {
				continue
			}
			c.checkNamespaceBodyStatement(stmt, nsType)
		}
	}

	// 4. Report pending overload signatures with no implementation (TS2391),
	// matching how checkBlockStatement validates block scopes. Skipped in
	// `declare namespace` bodies, where bodiless function declarations are
	// the norm in ambient contexts.
	if !node.Declare {
		for _, sigs := range bodyEnv.GetAllPendingOverloads() {
			for _, sig := range sigs {
				if sig.Name != nil {
					c.addError(sig.Name, "Function implementation is missing or not immediately following the declaration.")
				}
			}
		}
	}

	// 5. Restore env.
	c.env = outerEnv
}

// preprocessNamespaceTypes does an early pass over the body that processes
// type-only members (interfaces, type aliases, classes, nested namespaces) so
// they become available before function signatures are hoisted. This mirrors
// how the top-level checker handles these in Pass 1 before Pass 2 hoisting.
func (c *Checker) preprocessNamespaceTypes(body *parser.BlockStatement, nsType *types.NamespaceType) {
	process := func(inner parser.Statement, exported bool) {
		switch n := inner.(type) {
		case *parser.InterfaceDeclaration:
			c.checkInterfaceDeclaration(n)
			if exported && n.Name != nil {
				if t, found := c.env.ResolveType(n.Name.Value); found {
					nsType.TypeMembers[n.Name.Value] = t
				}
			}
		case *parser.TypeAliasStatement:
			c.checkTypeAliasStatement(n)
			if exported && n.Name != nil {
				if t, found := c.env.ResolveType(n.Name.Value); found {
					nsType.TypeMembers[n.Name.Value] = t
				}
			}
		case *parser.ClassDeclaration:
			c.checkClassDeclaration(n)
			if exported && n.Name != nil {
				if t, _, found := c.env.Resolve(n.Name.Value); found {
					nsType.ValueShape.Properties[n.Name.Value] = t
				}
				if t, found := c.env.ResolveType(n.Name.Value); found {
					nsType.TypeMembers[n.Name.Value] = t
				}
			}
		case *parser.NamespaceDeclaration:
			c.checkNamespaceDeclaration(n)
			if (exported || n.IsExported) && n.Name != nil {
				if childType, found := c.env.ResolveType(n.Name.Value); found {
					nsType.TypeMembers[n.Name.Value] = childType
					if childNs, ok := childType.(*types.NamespaceType); ok {
						nsType.ValueShape.Properties[n.Name.Value] = childNs.ValueShape
					}
				}
			}
		}
	}
	for _, stmt := range body.Statements {
		switch s := stmt.(type) {
		case *parser.InterfaceDeclaration, *parser.TypeAliasStatement, *parser.ClassDeclaration, *parser.NamespaceDeclaration:
			process(s, false)
		case *parser.ExportNamedDeclaration:
			if s.Declaration != nil {
				process(s.Declaration, true)
			}
		}
	}
}

// hoistNamespaceFunctions hoists function declarations inside a namespace body
// into the body env, mirroring how checkBlockStatement hoists function
// declarations. Unlike block hoisting, we also pick up `export function f(){}`
// (which the parser wraps in ExportNamedDeclaration and therefore does not
// place into HoistedDeclarations).
func (c *Checker) hoistNamespaceFunctions(body *parser.BlockStatement, env *Environment) {
	hoistFunc := func(funcLit *parser.FunctionLiteral) {
		if funcLit == nil || funcLit.Name == nil {
			return
		}
		fname := funcLit.Name.Value
		funcSig := c.resolveFunctionLiteralSignature(funcLit, env)
		if funcSig == nil {
			env.Define(fname, types.Any, false)
			return
		}
		funcObjectType := types.NewFunctionType(funcSig)
		env.Define(fname, funcObjectType, false)
		funcLit.SetComputedType(funcObjectType)
	}

	if body.HoistedDeclarations != nil {
		for _, hoistedNode := range body.HoistedDeclarations {
			if funcLit, ok := hoistedNode.(*parser.FunctionLiteral); ok {
				hoistFunc(funcLit)
			}
		}
	}

	// Also hoist `export function f(){}` declarations.
	for _, stmt := range body.Statements {
		exp, ok := stmt.(*parser.ExportNamedDeclaration)
		if !ok || exp.Declaration == nil {
			continue
		}
		exprStmt, ok := exp.Declaration.(*parser.ExpressionStatement)
		if !ok {
			continue
		}
		if funcLit, ok := exprStmt.Expression.(*parser.FunctionLiteral); ok {
			hoistFunc(funcLit)
		}
	}
}

// checkNamespaceBodyStatement processes a single statement inside a namespace body.
// If the statement is exported (either via `export ...` wrapper, or because it is
// a nested namespace declaration with IsExported=true), its bindings are copied
// into nsType.
func (c *Checker) checkNamespaceBodyStatement(stmt parser.Statement, nsType *types.NamespaceType) {
	exported := false
	inner := stmt
	if exp, ok := stmt.(*parser.ExportNamedDeclaration); ok && exp.Declaration != nil {
		exported = true
		inner = exp.Declaration
	}

	switch n := inner.(type) {
	case *parser.NamespaceDeclaration, *parser.InterfaceDeclaration, *parser.TypeAliasStatement, *parser.ClassDeclaration:
		// Already processed in preprocessNamespaceTypes.
		_ = n
		return

	case *parser.LetStatement:
		c.visit(n)
		if exported {
			c.copyVarBindings(n.Declarations, nsType)
		}
	case *parser.ConstStatement:
		c.visit(n)
		if exported {
			c.copyVarBindings(n.Declarations, nsType)
		}
	case *parser.VarStatement:
		c.visit(n)
		if exported {
			c.copyVarBindings(n.Declarations, nsType)
		}

	case *parser.ExpressionStatement:
		// Function declaration or enum declaration may show up here.
		if fn, ok := n.Expression.(*parser.FunctionLiteral); ok && fn.Name != nil {
			// Body checking — hoisting already defined the signature.
			c.visit(n)
			if exported {
				if t, _, found := c.env.Resolve(fn.Name.Value); found {
					nsType.ValueShape.Properties[fn.Name.Value] = t
				}
			}
			return
		}
		if enum, ok := n.Expression.(*parser.EnumDeclaration); ok && enum.Name != nil {
			c.checkEnumDeclaration(enum)
			if exported {
				if t, _, found := c.env.Resolve(enum.Name.Value); found {
					nsType.ValueShape.Properties[enum.Name.Value] = t
				}
				if t, found := c.env.ResolveType(enum.Name.Value); found {
					nsType.TypeMembers[enum.Name.Value] = t
				}
			}
			return
		}
		c.visit(n)

	default:
		c.visit(stmt)
	}

	if exported && stmt != inner {
		// nothing else to do; processing above already snapshotted exports.
		_ = fmt.Sprintf
	}
}

// copyVarBindings copies the resolved types of var/let/const declarators from
// the current body env into the namespace's ValueShape.
func (c *Checker) copyVarBindings(decls []*parser.VarDeclarator, nsType *types.NamespaceType) {
	for _, d := range decls {
		if d == nil || d.Name == nil {
			continue
		}
		if t, _, found := c.env.Resolve(d.Name.Value); found {
			nsType.ValueShape.Properties[d.Name.Value] = t
		}
	}
}

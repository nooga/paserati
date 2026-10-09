package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
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
	exportedDecl := c.nsDeclExported || node.IsExported
	c.nsDeclExported = false

	// 1. Get-or-create the NamespaceType in the current scope. We look in the
	//    type env: a NamespaceType always lives there.
	var nsType *types.NamespaceType
	if node.AmbientModule {
		// `declare module "m"` / `declare global`: check the body, bind no name.
		nsType = types.NewNamespaceType(name)
		nsType.Declare = true
	} else if existing, found := c.env.ResolveTypeLocal(name); found {
		if c.env.seededNamespaces[name] && !exportedDecl {
			// A non-exported namespace does not merge with the exported
			// namespace of the same name an earlier body of the parent
			// declared: it is a separate local declaration.
			delete(c.env.typeAliases, name)
			delete(c.env.symbols, name)
			delete(c.env.seededNamespaces, name)
		} else if ns, ok := existing.(*types.NamespaceType); ok {
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

	if parser.IsInstantiatedNamespace(node.Body) {
		nsType.Instantiated = true
	}

	// 2. Create an enclosed environment for the body.
	outerEnv := c.env
	// A namespace body is a var scope of its own, like a function body.
	bodyEnv := NewFunctionEnvironment(outerEnv)
	c.env = bodyEnv

	// Members of an ambient namespace are exported with or without the
	// `export` keyword.
	outerAmbient := c.inAmbientNamespace
	c.inAmbientNamespace = outerAmbient || node.Declare
	c.namespaceBodyDepth++
	defer func() {
		c.inAmbientNamespace = outerAmbient
		c.namespaceBodyDepth--
	}()

	// Seed bodyEnv with existing NAMESPACE children from nsType so that merge passes
	// find and reuse child namespaces. Only seed NamespaceType members — other types
	// (classes, interfaces, enums) should be free to re-declare in merge bodies.
	for childName, childT := range nsType.TypeMembers {
		if nsChild, ok := childT.(*types.NamespaceType); ok {
			bodyEnv.DefineTypeAlias(childName, nsChild)
			bodyEnv.Define(childName, nsChild.ValueShape, false)
			if bodyEnv.seededNamespaces == nil {
				bodyEnv.seededNamespaces = make(map[string]bool)
			}
			bodyEnv.seededNamespaces[childName] = true
		}
	}

	// 3. Walk each body statement and dispatch. We deliberately use a single-
	//    pass walk inside namespaces (good enough for our smoke tests; can be
	//    upgraded to multi-pass if needed later).
	if node.Body != nil {
		// Mirror the top-level checker passes:
		//   Pass A: interfaces, type aliases, classes, nested namespaces (types).
		//   Pass B: hoist function signatures (so interfaces are available for
		//           parameter type resolution).
		//   Pass C: visit remaining body statements.
		// While the top-level passes run, the body's value statements wait
		// until Pass 2.5: they may reference top-level functions, variables and
		// classes that are only declared by Pass 2. The queue slot is reserved
		// first so an enclosing namespace's body runs before its nested ones.
		slot := -1
		if c.deferMethodBodies && c.blockDepth == 0 && c.functionNestingDepth == 0 {
			slot = len(c.deferredMethodBodies)
			c.deferredMethodBodies = append(c.deferredMethodBodies, deferredMethodBodyCheck{})
		}
		c.preprocessNamespaceTypes(node.Body, nsType)
		// Names the body's var statements declare must exist while the
		// function signatures are hoisted (`typeof a` in a parameter type).
		c.hoistAnnotatedNamespaceVars(node.Body, bodyEnv)
		c.hoistVarNames(bodyEnv, node.Body.Statements)
		c.hoistNamespaceFunctions(node.Body, bodyEnv, nsType)

		ambientBody := c.inAmbientNamespace
		runBody := func() {
			// The body may run after checkNamespaceDeclaration returned (it is
			// deferred to Pass 2.5), so its ambient/namespace context is
			// re-established here.
			prevEnv := c.env
			prevAmbient := c.inAmbientNamespace
			c.env = bodyEnv
			c.inAmbientNamespace = ambientBody
			c.namespaceBodyDepth++
			defer func() {
				c.inAmbientNamespace = prevAmbient
				c.namespaceBodyDepth--
			}()
			// Exported values of earlier declarations of this namespace are
			// visible by bare name in a merged body. (Done here, not before
			// the body is queued: earlier bodies publish their exports when
			// they run.)
			for valueName, valueType := range nsType.ValueShape.Properties {
				if _, isNs := nsType.TypeMembers[valueName].(*types.NamespaceType); isNs {
					continue
				}
				bodyEnv.Define(valueName, valueType, false)
			}
			c.hoistVarNames(bodyEnv, node.Body.Statements)
			c.preRegisterBlockScoped(node.Body.Statements)
			for _, stmt := range node.Body.Statements {
				if stmt == nil {
					continue
				}
				c.checkNamespaceBodyStatement(stmt, nsType)
			}
			c.checkNamespaceOverloads(node, bodyEnv)
			c.env = prevEnv
		}
		if slot >= 0 {
			c.deferredMethodBodies[slot] = deferredMethodBodyCheck{env: bodyEnv, run: runBody}
		} else {
			runBody()
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
					nsType.ValueShape.SetProperty(n.Name.Value, t)
				}
				if t, found := c.env.ResolveType(n.Name.Value); found {
					nsType.TypeMembers[n.Name.Value] = t
				}
			}
		case *parser.ExpressionStatement:
			// Enums are declarations too: they must be known before the
			// namespace's value statements (and outside type annotations like
			// `A.Color`) are checked.
			if enum, ok := n.Expression.(*parser.EnumDeclaration); ok && enum != nil && enum.Name != nil {
				if exported {
					// An exported enum merges with the same-named exported enum
					// of an earlier body of this namespace.
					if prev, ok := nsType.ValueShape.Properties[enum.Name.Value].(*types.EnumType); ok {
						if _, local := c.env.symbols[enum.Name.Value]; !local {
							c.env.Define(enum.Name.Value, prev, false)
						}
					}
				}
				c.checkEnumDeclaration(enum)
				if exported {
					if t, _, found := c.env.Resolve(enum.Name.Value); found {
						nsType.ValueShape.SetProperty(enum.Name.Value, t)
					}
					if t, found := c.env.ResolveType(enum.Name.Value); found {
						nsType.TypeMembers[enum.Name.Value] = t
					}
				}
			}
		case *parser.NamespaceDeclaration:
			c.nsDeclExported = exported
			c.checkNamespaceDeclaration(n)
			if (exported || n.IsExported) && n.Name != nil {
				if childType, found := c.env.ResolveType(n.Name.Value); found {
					nsType.TypeMembers[n.Name.Value] = childType
					if childNs, ok := childType.(*types.NamespaceType); ok {
						nsType.ValueShape.SetProperty(n.Name.Value, childNs.ValueShape)
					}
				}
			}
		}
	}
	for _, stmt := range body.Statements {
		switch s := stmt.(type) {
		case *parser.InterfaceDeclaration, *parser.TypeAliasStatement, *parser.ClassDeclaration, *parser.NamespaceDeclaration, *parser.ExpressionStatement:
			process(s, c.inAmbientNamespace)
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
func (c *Checker) hoistNamespaceFunctions(body *parser.BlockStatement, env *Environment, nsType *types.NamespaceType) {
	hoistFunc := func(funcLit *parser.FunctionLiteral, exported bool) {
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
		// Exported functions are visible through the namespace right away, even
		// while the body's value statements are still waiting to be checked.
		if exported && nsType != nil {
			nsType.ValueShape.SetProperty(fname, funcObjectType)
		}
	}

	if body.HoistedDeclarations != nil {
		for _, hoistedNode := range body.HoistedDeclarations {
			if funcLit, ok := hoistedNode.(*parser.FunctionLiteral); ok {
				hoistFunc(funcLit, false)
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
			hoistFunc(funcLit, true)
		}
	}
}

// checkNamespaceBodyStatement processes a single statement inside a namespace body.
// If the statement is exported (either via `export ...` wrapper, or because it is
// a nested namespace declaration with IsExported=true), its bindings are copied
// into nsType.
func (c *Checker) checkNamespaceBodyStatement(stmt parser.Statement, nsType *types.NamespaceType) {
	exported := c.inAmbientNamespace
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

	case *parser.LetStatement, *parser.ConstStatement, *parser.VarStatement,
		*parser.ObjectDestructuringDeclaration, *parser.ArrayDestructuringDeclaration:
		c.visit(inner)
		if exported {
			// Every declarator of the clause, and every name a destructuring
			// pattern binds - `export const {a} = obj` had no case here at all,
			// so a was missing from the namespace type (TS2339 on N.a) even
			// though it existed as a binding inside the body.
			c.copyBindingTypes(parser.DeclaredNames(inner), nsType)
		}

	case *parser.FunctionSignature:
		// `declare function f(...)` inside an ambient namespace.
		if c.inAmbientNamespace {
			n.Declare = true
		}
		c.visit(inner)
		if exported && n.Name != nil {
			if t, _, found := c.env.Resolve(n.Name.Value); found {
				nsType.ValueShape.SetProperty(n.Name.Value, t)
			}
		}

	case *parser.ExpressionStatement:
		if sig, ok := n.Expression.(*parser.FunctionSignature); ok && sig.Name != nil {
			if c.inAmbientNamespace {
				sig.Declare = true
			}
			c.visit(n)
			if exported {
				if t, _, found := c.env.Resolve(sig.Name.Value); found {
					nsType.ValueShape.SetProperty(sig.Name.Value, t)
				}
			}
			return
		}
		// Function declaration or enum declaration may show up here.
		if fn, ok := n.Expression.(*parser.FunctionLiteral); ok && fn.Name != nil {
			// Body checking — hoisting already defined the signature.
			c.visit(n)
			if exported {
				if t, _, found := c.env.Resolve(fn.Name.Value); found {
					nsType.ValueShape.SetProperty(fn.Name.Value, t)
				}
			}
			return
		}
		if enum, ok := n.Expression.(*parser.EnumDeclaration); ok && enum.Name != nil {
			return // already processed in preprocessNamespaceTypes
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

// copyBindingTypes copies the resolved types of the named bindings from the
// current body env into the namespace's ValueShape.
func (c *Checker) copyBindingTypes(names []string, nsType *types.NamespaceType) {
	for _, name := range names {
		if t, _, found := c.env.Resolve(name); found {
			nsType.ValueShape.SetProperty(name, t)
		}
	}
}

// checkNamespaceOverloads reports pending overload signatures with no
// implementation (TS2391), matching how checkBlockStatement validates block
// scopes. Skipped in `declare namespace` bodies, where bodiless function
// declarations are the norm in ambient contexts.
func (c *Checker) checkNamespaceOverloads(node *parser.NamespaceDeclaration, bodyEnv *Environment) {
	if node.Declare {
		return
	}
	for _, sigs := range bodyEnv.GetAllPendingOverloads() {
		for _, sig := range sigs {
			if sig.Name != nil {
				c.addErrorWithCode(sig.Name, errors.TS2391, "Function implementation is missing or not immediately following the declaration.")
			}
		}
	}
}

// hoistAnnotatedNamespaceVars declares the annotated top-level `var`s of a
// namespace body with their declared types before function signatures are
// hoisted, so `typeof a` in a parameter type sees `a`'s annotation.
func (c *Checker) hoistAnnotatedNamespaceVars(body *parser.BlockStatement, env *Environment) {
	for _, stmt := range body.Statements {
		if exp, ok := stmt.(*parser.ExportNamedDeclaration); ok && exp.Declaration != nil {
			stmt = exp.Declaration
		}
		vs, ok := stmt.(*parser.VarStatement)
		if !ok {
			continue
		}
		for _, d := range vs.Declarations {
			if d == nil || d.Name == nil || d.TypeAnnotation == nil {
				continue
			}
			if _, found := env.symbols[d.Name.Value]; found {
				continue
			}
			if t := c.resolveTypeAnnotation(d.TypeAnnotation); t != nil {
				env.Define(d.Name.Value, t, false)
			}
		}
	}
}

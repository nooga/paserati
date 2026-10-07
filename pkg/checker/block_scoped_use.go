package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// Block-scoped names (let/const/class) are in their temporal dead zone from
// the start of their scope until the declaration runs. tsc reports a use that
// precedes the declaration (TS2448 for variables, TS2449 for classes) when
// that use is not deferred - i.e. not inside a function body that merely
// closes over the name - and so does a use inside the declaration's own
// initializer (`let x = x`).

type blockScopedKind int

const (
	bsVariable blockScopedKind = iota
	bsClass
)

type blockScopedInfo struct {
	pos          int // source offset of the declared name
	kind         blockScopedKind
	fnDepth      int // functionNestingDepth where the name is declared
	initializing bool
	constInit    parser.Expression // initializer of a `const`, for constant evaluation (enum members)
}

// declareBlockScoped records where a let/const/class name is declared.
func (c *Checker) declareBlockScoped(env *Environment, name string, tok *lexer.Token, kind blockScopedKind) *blockScopedInfo {
	if env == nil || tok == nil || name == "" {
		return nil
	}
	if c.blockScoped == nil {
		c.blockScoped = make(map[*Environment]map[string]*blockScopedInfo)
	}
	m := c.blockScoped[env]
	if m == nil {
		m = make(map[string]*blockScopedInfo)
		c.blockScoped[env] = m
	}
	if existing := m[name]; existing != nil {
		return existing // first declaration wins; duplicates are the binder's business
	}
	info := &blockScopedInfo{pos: tok.StartPos, kind: kind, fnDepth: c.functionNestingDepth}
	m[name] = info
	return info
}

// blockScopedInfoFor finds the record for name as seen from the current scope.
func (c *Checker) blockScopedInfoFor(name string) *blockScopedInfo {
	if c.blockScoped == nil {
		return nil
	}
	for e := c.env; e != nil; e = e.outer {
		if m := c.blockScoped[e]; m != nil {
			if info := m[name]; info != nil {
				return info
			}
		}
		if _, isVal := e.symbols[name]; isVal {
			return nil // a binding without a TDZ record (var, parameter, ...) shadows outer ones
		}
	}
	return nil
}

// preRegisterBlockScoped records the let/const/class names a statement list
// declares at its own level, so a use that precedes the declaration is found
// (and reported as TS2448/TS2449) instead of being an unresolved name.
func (c *Checker) preRegisterBlockScoped(stmts []parser.Statement) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *parser.LetStatement:
			if !n.Declare {
				for _, d := range n.Declarations {
					if d != nil && d.Name != nil {
						c.declareBlockScoped(c.env, d.Name.Value, d.Name.Token, bsVariable)
					}
				}
			}
		case *parser.ConstStatement:
			if !n.Declare {
				for _, d := range n.Declarations {
					if d != nil && d.Name != nil {
						if info := c.declareBlockScoped(c.env, d.Name.Value, d.Name.Token, bsVariable); info != nil && info.constInit == nil {
							info.constInit = d.Value
						}
					}
				}
			}
		case *parser.ClassDeclaration:
			if n != nil && n.Name != nil && !n.Declare {
				c.declareBlockScoped(c.env, n.Name.Value, n.Name.Token, bsClass)
			}
		}
	}
}

// reportUnresolvedOrTDZ reports an identifier that did not resolve: a name that
// is declared later in this scope is a TDZ error, anything else is TS2304/2552.
func (c *Checker) reportUnresolvedOrTDZ(node *parser.Identifier) {
	if info := c.blockScopedInfoFor(node.Value); info != nil && node.Token != nil {
		if c.functionNestingDepth > info.fnDepth || c.deferredContextDepth > 0 {
			return // used inside a function that runs later: the declaration will be there
		}
		if node.Token.StartPos < info.pos || info.initializing {
			c.reportBlockScopedUse(node, info)
			return
		}
		// Declared earlier in this scope but not defined yet: an early pass
		// (class members, enums) is looking at it before Pass 2 hoists it.
		return
	}
	// Top-level var and function declarations are hoisted by Pass 2; class
	// members, enums and namespace bodies are checked in Pass 1, before that.
	if c.deferMethodBodies && c.programHoistedNames[node.Value] {
		return
	}
	if c.isTypeOnlyName(node.Value) {
		c.addErrorWithCode(node, "TS2693", fmt.Sprintf("'%s' only refers to a type, but is being used as a value here.", node.Value))
		return
	}
	c.addCannotFindNameError(node, c.env, node.Value)
}

// isTypeOnlyName reports whether name denotes a type (interface, alias, type
// parameter, primitive keyword) but no value.
func (c *Checker) isTypeOnlyName(name string) bool {
	switch name {
	case "string", "number", "boolean", "symbol", "bigint", "object", "void", "never", "unknown", "any":
		return true
	}
	if _, ok := c.env.ResolveTypeParameter(name); ok {
		return true
	}
	if _, ok := c.env.ResolveType(name); ok {
		return true
	}
	return false
}

func (c *Checker) reportBlockScopedUse(node *parser.Identifier, info *blockScopedInfo) {
	if info.kind == bsClass {
		c.addErrorWithCode(node, tsClassUsedBefore, fmt.Sprintf("Class '%s' used before its declaration.", node.Value))
		return
	}
	c.addErrorWithCode(node, tsBlockScopedUsedBefore, fmt.Sprintf("Block-scoped variable '%s' used before its declaration.", node.Value))
}

// checkBlockScopedUse reports a use of a let/const/class name before its
// declaration. node is the identifier being resolved.
func (c *Checker) checkBlockScopedUse(node *parser.Identifier) {
	if node == nil || node.Token == nil {
		return
	}
	info := c.blockScopedInfoFor(node.Value)
	if info == nil {
		return
	}
	if c.functionNestingDepth > info.fnDepth || c.deferredContextDepth > 0 {
		return // inside a function (or instance property initializer): runs later
	}
	if node.Token.StartPos >= info.pos && !info.initializing {
		return
	}
	c.reportBlockScopedUse(node, info)
}

// reportMissingSuperclass reports an unresolvable `extends` identifier: TS2449
// when it names a class declared later in the file, TS2304 otherwise.
func (c *Checker) reportMissingSuperclass(ident *parser.Identifier) {
	if info := c.blockScopedInfoFor(ident.Value); info != nil {
		if info.kind == bsClass {
			c.addErrorWithCode(ident, tsClassUsedBefore, fmt.Sprintf("Class '%s' used before its declaration.", ident.Value))
			return
		}
		// A let/const of that name: before its declaration it is TS2448; after
		// it, it just has not been defined yet by the passes that run later.
		if ident.Token != nil && ident.Token.StartPos < info.pos {
			c.reportBlockScopedUse(ident, info)
		}
		return
	}
	if c.deferMethodBodies && c.programHoistedNames[ident.Value] {
		return // a hoisted var/function the later pass will define
	}
	c.addCannotFindNameError(ident, c.env, ident.Value)
}

// isAmbientVarLike reports `declare let/const/var` statements.
func isAmbientVarLike(stmt parser.Statement) bool {
	switch n := stmt.(type) {
	case *parser.LetStatement:
		return n.Declare
	case *parser.ConstStatement:
		return n.Declare
	case *parser.VarStatement:
		return n.Declare
	}
	return false
}

// beginLoopHeadTDZ marks the let/const names a for-in/of head declares as being
// in their TDZ while the iterated expression is evaluated; the returned func
// ends it.
func (c *Checker) beginLoopHeadTDZ(head parser.Statement) func() {
	var infos []*blockScopedInfo
	var names []string
	switch n := head.(type) {
	case *parser.LetStatement:
		for _, d := range n.Declarations {
			if d != nil && d.Name != nil {
				names = append(names, d.Name.Value)
			}
		}
	case *parser.ConstStatement:
		for _, d := range n.Declarations {
			if d != nil && d.Name != nil {
				names = append(names, d.Name.Value)
			}
		}
	}
	if m := c.blockScoped[c.env]; m != nil {
		for _, name := range names {
			if info := m[name]; info != nil && !info.initializing {
				info.initializing = true
				infos = append(infos, info)
			}
		}
	}
	return func() {
		for _, info := range infos {
			info.initializing = false
		}
	}
}

// preRegisterTopLevelBlockScoped is preRegisterBlockScoped for a program's own
// statement list (including exported declarations), run before any pass so the
// constant initializers of top-level consts are known to enum evaluation.
func (c *Checker) preRegisterTopLevelBlockScoped(stmts []parser.Statement) {
	plain := make([]parser.Statement, 0, len(stmts))
	for _, s := range stmts {
		if e, ok := s.(*parser.ExportNamedDeclaration); ok && e != nil && e.Declaration != nil {
			plain = append(plain, e.Declaration)
			continue
		}
		plain = append(plain, s)
	}
	c.preRegisterBlockScoped(plain)
}

// checkNamespaceUsedAsValue handles an identifier in a value position that
// names a namespace with no runtime value (only types). Such a namespace has no
// value meaning, so name resolution looks past it to an outer value of the same
// name (a lib global like Symbol); only when there is none is it TS2708. It
// returns the type the identifier should have.
func (c *Checker) checkNamespaceUsedAsValue(node *parser.Identifier, valueType types.Type) types.Type {
	if node == nil || valueType == nil {
		return valueType
	}
	nsT, found := c.env.ResolveType(node.Value)
	if !found {
		return valueType
	}
	ns, ok := nsT.(*types.NamespaceType)
	if !ok || ns.Instantiated || ns.Declare || ns.ValueShape != valueType || c.isLibGlobalValue(node.Value) {
		return valueType
	}
	// Find the scope that owns the namespace's value binding, then look outward.
	for e := c.env; e != nil; e = e.outer {
		if info, ok := e.symbols[node.Value]; ok && info.Type == valueType {
			if e.outer != nil {
				if outerType, _, ok := e.outer.Resolve(node.Value); ok {
					return outerType
				}
			}
			break
		}
	}
	c.addErrorWithCode(node, "TS2708", fmt.Sprintf("Cannot use namespace '%s' as a value.", node.Value))
	return valueType
}

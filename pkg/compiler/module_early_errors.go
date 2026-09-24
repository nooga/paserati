package compiler

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// Early errors that apply to Module code (ECMA-262 16.2.1.1 and the rules
// that make module code strict with `await` reserved). They are reported
// before any import is loaded, so a module that is not valid source text is
// a SyntaxError in the parse phase, never a resolution failure.

// isModuleGoal reports whether this compilation is a module's top level:
// module mode, and not eval code, Function() code or a nested function.
func (c *Compiler) isModuleGoal() bool {
	return c.IsModuleMode() && !c.forceScriptMode && !c.isIndirectEval &&
		c.callerScopeDesc == nil && c.enclosing == nil
}

// checkModuleEarlyErrors reports the first module early error in program.
func (c *Compiler) checkModuleEarlyErrors(program *parser.Program) {
	m := &moduleEarlyErrors{c: c}
	m.checkDeclarations(program)
	if m.failed {
		return
	}
	m.checkExports(program)
	if m.failed {
		return
	}
	for _, stmt := range program.Statements {
		m.walk(stmt, walkCtx{topLevel: true, awaitOK: true})
		if m.failed {
			return
		}
	}
}

type moduleEarlyErrors struct {
	c      *Compiler
	failed bool
}

func (m *moduleEarlyErrors) errorf(node parser.Node, format string, args ...interface{}) {
	if m.failed {
		return
	}
	m.failed = true
	m.c.addError(node, "SyntaxError: "+fmt.Sprintf(format, args...))
}

// --- Declarations ---------------------------------------------------------

// checkDeclarations enforces that the module's LexicallyDeclaredNames have
// no duplicates and none of them is also a VarDeclaredName. In a module,
// top-level function declarations and import bindings are lexical.
func (m *moduleEarlyErrors) checkDeclarations(program *parser.Program) {
	lexical := make(map[string]bool)
	declareLexical := func(node parser.Node, name string) {
		if name == "" {
			return
		}
		if lexical[name] {
			m.errorf(node, "Identifier '%s' has already been declared", name)
			return
		}
		lexical[name] = true
	}
	for _, stmt := range program.Statements {
		for _, d := range moduleLexicalDeclarations(stmt) {
			declareLexical(d.node, d.name)
			if m.failed {
				return
			}
		}
	}
	for _, name := range collectVarDeclarations(program.Statements) {
		if lexical[name] {
			m.errorf(program, "Identifier '%s' has already been declared", name)
			return
		}
	}
}

type declaredName struct {
	node parser.Node
	name string
}

// moduleLexicalDeclarations lists the lexical bindings one top-level module
// item declares: let/const, classes, function declarations, the name of a
// named `export default` function/class, and import bindings. TypeScript-only
// declarations (types, interfaces, enums, namespaces, overload signatures and
// `declare` forms) may merge with values and are left out.
func moduleLexicalDeclarations(stmt parser.Statement) []declaredName {
	var out []declaredName
	add := func(id *parser.Identifier) {
		if id != nil {
			out = append(out, declaredName{id, id.Value})
		}
	}
	addNames := func(node parser.Node, s parser.Statement) {
		for _, n := range parser.DeclaredNames(s) {
			out = append(out, declaredName{node, n})
		}
	}
	switch s := stmt.(type) {
	case *parser.LetStatement:
		if !s.Declare {
			addNames(s, s)
		}
	case *parser.ConstStatement:
		if !s.Declare {
			addNames(s, s)
		}
	case *parser.ObjectDestructuringDeclaration:
		if s.Token.Literal == "let" || s.Token.Literal == "const" {
			addNames(s, s)
		}
	case *parser.ArrayDestructuringDeclaration:
		if s.Token.Literal == "let" || s.Token.Literal == "const" {
			addNames(s, s)
		}
	case *parser.DeclarationGroup:
		for _, d := range s.Declarations {
			out = append(out, moduleLexicalDeclarations(d)...)
		}
	case *parser.ClassDeclaration:
		if !s.Declare {
			add(s.Name)
		}
	case *parser.ExpressionStatement:
		if fn := functionDeclaration(s); fn != nil {
			add(fn.Name)
		}
	case *parser.ExportNamedDeclaration:
		if s.Declaration != nil && !s.IsTypeOnly {
			out = append(out, moduleLexicalDeclarations(s.Declaration)...)
		}
	case *parser.ExportDefaultDeclaration:
		if s.IsDeclaration {
			switch d := s.Declaration.(type) {
			case *parser.FunctionLiteral:
				add(d.Name)
			case *parser.ClassExpression:
				add(d.Name)
			}
		}
	case *parser.ImportDeclaration:
		if s.IsTypeOnly {
			break
		}
		for _, spec := range s.Specifiers {
			switch sp := spec.(type) {
			case *parser.ImportDefaultSpecifier:
				add(sp.Local)
			case *parser.ImportNamedSpecifier:
				if !sp.IsTypeOnly {
					add(sp.Local)
				}
			case *parser.ImportNamespaceSpecifier:
				add(sp.Local)
			}
		}
	}
	return out
}

// functionDeclaration returns the function a statement declares, if it is a
// function declaration with a body (not an expression statement or an
// overload signature).
func functionDeclaration(s *parser.ExpressionStatement) *parser.FunctionLiteral {
	fn, ok := s.Expression.(*parser.FunctionLiteral)
	if !ok || fn.Name == nil || fn.Parenthesized || fn.Body == nil {
		return nil
	}
	return fn
}

// --- Exports --------------------------------------------------------------

// checkExports enforces: ExportedNames has no duplicates; every local name
// exported without a `from` clause is declared in the module and is an
// identifier; module export names are well-formed Unicode; and import
// bindings are not `eval`/`arguments`.
func (m *moduleEarlyErrors) checkExports(program *parser.Program) {
	declared := make(map[string]bool)
	for _, stmt := range program.Statements {
		for _, d := range moduleLexicalDeclarations(stmt) {
			declared[d.name] = true
		}
		collectTypeScriptDeclaredNames(stmt, declared)
	}
	for _, name := range collectVarDeclarations(program.Statements) {
		declared[name] = true
	}

	exported := make(map[string]bool)
	exportName := func(node parser.Node, name string) {
		if exported[name] {
			m.errorf(node, "Duplicate export of '%s'", name)
			return
		}
		exported[name] = true
	}

	for _, stmt := range program.Statements {
		if m.failed {
			return
		}
		switch s := stmt.(type) {
		case *parser.ImportDeclaration:
			if s.IsTypeOnly {
				continue
			}
			for _, spec := range s.Specifiers {
				switch sp := spec.(type) {
				case *parser.ImportDefaultSpecifier:
					m.checkImportBinding(sp.Local)
				case *parser.ImportNamedSpecifier:
					m.checkModuleExportName(sp.Imported, sp.Imported.Value)
					m.checkImportBinding(sp.Local)
				case *parser.ImportNamespaceSpecifier:
					m.checkImportBinding(sp.Local)
				}
			}
		case *parser.ExportDefaultDeclaration:
			exportName(s, "default")
		case *parser.ExportAllDeclaration:
			if s.IsTypeOnly || s.Exported == nil {
				continue
			}
			name := moduleExportNameValue(s.Exported)
			m.checkModuleExportName(s.Exported, name)
			exportName(s.Exported, name)
		case *parser.ExportNamedDeclaration:
			if s.IsTypeOnly {
				continue
			}
			if s.Declaration != nil {
				for _, d := range exportedDeclarationNames(s.Declaration) {
					exportName(d.node, d.name)
				}
				continue
			}
			for _, spec := range s.Specifiers {
				sp, ok := spec.(*parser.ExportNamedSpecifier)
				if !ok {
					continue
				}
				name := moduleExportNameValue(sp.Exported)
				m.checkModuleExportName(sp.Exported, name)
				if s.Source != nil {
					m.checkModuleExportName(sp.Local, moduleExportNameValue(sp.Local))
				} else if lit, isString := sp.Local.(*parser.StringLiteral); isString {
					m.errorf(lit, "A string literal cannot be used as an exported binding without 'from'")
				} else if id, isIdent := sp.Local.(*parser.Identifier); isIdent && !declared[id.Value] {
					m.errorf(id, "Export '%s' is not defined in module", id.Value)
				}
				exportName(sp.Exported, name)
				if m.failed {
					return
				}
			}
		}
	}
}

// exportedDeclarationNames lists the names an `export <declaration>` exports.
func exportedDeclarationNames(decl parser.Statement) []declaredName {
	switch d := decl.(type) {
	case *parser.VarStatement:
		if d.Declare {
			return nil
		}
		var out []declaredName
		for _, n := range parser.DeclaredNames(d) {
			out = append(out, declaredName{d, n})
		}
		return out
	case *parser.ObjectDestructuringDeclaration, *parser.ArrayDestructuringDeclaration:
		var out []declaredName
		for _, n := range parser.DeclaredNames(decl) {
			out = append(out, declaredName{decl, n})
		}
		return out
	}
	return moduleLexicalDeclarations(decl)
}

// collectTypeScriptDeclaredNames records names bound by TypeScript-only
// declarations, which `export { X }` may also refer to.
func collectTypeScriptDeclaredNames(stmt parser.Statement, declared map[string]bool) {
	mark := func(id *parser.Identifier) {
		if id != nil {
			declared[id.Value] = true
		}
	}
	switch s := stmt.(type) {
	case *parser.InterfaceDeclaration:
		mark(s.Name)
	case *parser.TypeAliasStatement:
		mark(s.Name)
	case *parser.EnumDeclaration:
		mark(s.Name)
	case *parser.NamespaceDeclaration:
		mark(s.Name)
	case *parser.FunctionSignature:
		mark(s.Name)
	case *parser.ClassDeclaration:
		mark(s.Name)
	case *parser.LetStatement, *parser.ConstStatement, *parser.VarStatement:
		for _, n := range parser.DeclaredNames(s) {
			declared[n] = true
		}
	case *parser.ExportNamedDeclaration:
		if s.Declaration != nil {
			collectTypeScriptDeclaredNames(s.Declaration, declared)
		}
	}
}

func moduleExportNameValue(e parser.Expression) string {
	switch n := e.(type) {
	case *parser.Identifier:
		return n.Value
	case *parser.StringLiteral:
		return n.Value
	}
	return ""
}

// checkModuleExportName rejects a string ModuleExportName that is not
// well-formed Unicode (contains a lone surrogate).
func (m *moduleEarlyErrors) checkModuleExportName(node parser.Node, name string) {
	switch n := node.(type) {
	case *parser.StringLiteral:
	case *parser.Identifier:
		// Import specifiers keep a string name in an Identifier.
		if n.Token == nil || n.Token.Type != lexer.STRING {
			return
		}
	default:
		return
	}
	units := vm.StringToUTF16(name)
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u >= 0xD800 && u <= 0xDBFF:
			if i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF {
				i++
				continue
			}
			m.errorf(node, "Module export name is not well-formed Unicode")
			return
		case u >= 0xDC00 && u <= 0xDFFF:
			m.errorf(node, "Module export name is not well-formed Unicode")
			return
		}
	}
}

// checkImportBinding applies the strict-mode rule that `eval` and
// `arguments` cannot be bound.
func (m *moduleEarlyErrors) checkImportBinding(id *parser.Identifier) {
	if id != nil && (id.Value == "eval" || id.Value == "arguments") {
		m.errorf(id, "Unexpected eval or arguments in strict mode")
	}
}

// --- Syntax-directed walk -------------------------------------------------

// walkCtx is the context a node appears in.
type walkCtx struct {
	topLevel   bool // directly in the module's StatementList
	inFunction bool // inside a non-arrow function, method, field initializer or static block
	inArrow    bool // inside an arrow function (return is allowed)
	awaitOK    bool // an await expression is allowed (module top level, async function bodies)
}

// walk visits node and its descendants, reporting import/export declarations
// that are not ModuleItems, and new.target, super and return at the module
// top level, and uses of `await` (reserved in module code) and `yield`
// (reserved in strict code) as identifiers.
func (m *moduleEarlyErrors) walk(node parser.Node, ctx walkCtx) {
	if m.failed || isNilASTNode(node) {
		return
	}
	isTop := ctx.topLevel
	ctx.topLevel = false
	inner := ctx
	switch n := node.(type) {
	case *parser.ImportDeclaration:
		if !isTop {
			m.errorf(n, "Cannot use import statement outside the module top level")
		}
		return
	case *parser.ExportNamedDeclaration:
		if !isTop {
			m.errorf(n, "Unexpected token 'export'")
			return
		}
		if n.Declaration != nil {
			m.walk(n.Declaration, ctx)
		}
		return
	case *parser.ExportAllDeclaration:
		if !isTop {
			m.errorf(n, "Unexpected token 'export'")
		}
		return
	case *parser.ExportDefaultDeclaration:
		if !isTop {
			m.errorf(n, "Unexpected token 'export'")
			return
		}
		m.walk(n.Declaration, ctx)
		return
	case *parser.DeclarationGroup:
		// A flattened multi-declarator statement: still a module item.
		for _, d := range n.Declarations {
			inner := ctx
			inner.topLevel = isTop
			m.walk(d, inner)
		}
		return
	case *parser.NamespaceDeclaration, *parser.InterfaceDeclaration, *parser.TypeAliasStatement,
		*parser.EnumDeclaration, *parser.FunctionSignature:
		// TypeScript declarations have their own rules (namespaces may export).
		return
	case *parser.NewTargetExpression:
		if !ctx.inFunction {
			m.errorf(n, "new.target expression is not allowed here")
		}
		return
	case *parser.SuperExpression:
		if !ctx.inFunction {
			m.errorf(n, "'super' keyword unexpected here")
		}
		return
	case *parser.ReturnStatement:
		if !ctx.inFunction && !ctx.inArrow {
			m.errorf(n, "Illegal return statement")
			return
		}
	case *parser.Identifier:
		m.checkIdentifier(n)
		return
	case *parser.LabeledStatement:
		m.checkIdentifier(n.Label)
		m.walk(n.Statement, ctx)
		return
	case *parser.FunctionLiteral:
		m.walk(n.Name, ctx)
		params := walkCtx{inFunction: true}
		body := params
		body.awaitOK = n.IsAsync
		m.walkFunctionParts(n.Parameters, n.RestParameter, n.Body, params, body)
		return
	case *parser.ShorthandMethod:
		// The node does not record whether the method is async, so await is
		// not checked in it.
		inner = walkCtx{inFunction: true, awaitOK: true}
		m.walkFunctionParts(n.Parameters, n.RestParameter, n.Body, inner, inner)
		return
	case *parser.ArrowFunctionLiteral:
		params := walkCtx{inFunction: ctx.inFunction, inArrow: true}
		body := params
		body.awaitOK = n.IsAsync
		m.walkFunctionParts(n.Parameters, n.RestParameter, n.Body, params, body)
		return
	case *parser.AwaitExpression:
		if !ctx.awaitOK {
			m.errorf(n, "await is only valid in async functions and the top level bodies of modules")
			return
		}
	case *parser.ClassDeclaration:
		m.walk(n.Name, ctx)
		m.walk(n.SuperClass, ctx)
		m.walkClassBody(n.Body, ctx)
		return
	case *parser.ClassExpression:
		m.walk(n.Name, ctx)
		m.walk(n.SuperClass, ctx)
		m.walkClassBody(n.Body, ctx)
		return
	case *parser.MemberExpression:
		m.walk(n.Object, ctx)
		m.walkPropertyKey(n.Property, ctx)
		return
	case *parser.OptionalChainingExpression:
		m.walk(n.Object, ctx)
		m.walkPropertyKey(n.Property, ctx)
		m.walk(n.Continuation, ctx)
		return
	case *parser.ObjectLiteral:
		for _, p := range n.Properties {
			if p == nil {
				continue
			}
			if p.Value == nil {
				// Shorthand or spread: the key is a reference.
				m.walk(p.Key, ctx)
			} else {
				m.walkPropertyKey(p.Key, ctx)
				m.walk(p.Value, ctx)
			}
		}
		return
	}
	walkChildren(node, func(child parser.Node) { m.walk(child, inner) })
}

func (m *moduleEarlyErrors) walkFunctionParts(params []*parser.Parameter, rest *parser.RestParameter, body parser.Node, paramCtx, bodyCtx walkCtx) {
	for _, p := range params {
		if p != nil {
			m.walk(p.Name, paramCtx)
			m.walk(p.Pattern, paramCtx)
			m.walk(p.DefaultValue, paramCtx)
		}
	}
	if rest != nil {
		m.walk(rest.Name, paramCtx)
		m.walk(rest.Pattern, paramCtx)
	}
	m.walk(body, bodyCtx)
}

func (m *moduleEarlyErrors) walkClassBody(body *parser.ClassBody, ctx walkCtx) {
	if body == nil {
		return
	}
	member := walkCtx{inFunction: true}
	for _, md := range body.Methods {
		if md != nil {
			m.walkPropertyKey(md.Key, ctx)
			m.walk(md.Value, member)
		}
	}
	for _, pd := range body.Properties {
		if pd != nil {
			m.walkPropertyKey(pd.Key, ctx)
			m.walk(pd.Value, member)
		}
	}
	for _, b := range body.StaticInitializers {
		m.walk(b, member)
	}
}

// walkPropertyKey visits a property name only when it is computed; a plain
// IdentifierName key may be any name, reserved words included.
func (m *moduleEarlyErrors) walkPropertyKey(key parser.Expression, ctx walkCtx) {
	if cpn, ok := key.(*parser.ComputedPropertyName); ok {
		m.walk(cpn.Expr, ctx)
	}
}

func (m *moduleEarlyErrors) checkIdentifier(id *parser.Identifier) {
	if id == nil || id.Token == nil {
		return
	}
	switch id.Value {
	case "await":
		m.errorf(id, "Unexpected reserved word 'await' in module code")
	case "yield":
		m.errorf(id, "Unexpected strict mode reserved word 'yield'")
	}
}

// --- Generic child enumeration --------------------------------------------

func isNilASTNode(node parser.Node) bool {
	if node == nil {
		return true
	}
	v := reflect.ValueOf(node)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

type childField struct {
	index        int
	propertyName bool // a Key/Property field: only a computed key holds code
}

var (
	parserPkgPath = reflect.TypeOf(parser.Program{}).PkgPath()
	childFieldsMu sync.RWMutex
	childFields   = map[reflect.Type][]childField{}
)

// skippedChildFields are AST fields that hold no runtime code: tokens and
// TypeScript type syntax.
var skippedChildFields = map[string]bool{
	"Token": true, "TypeAnnotation": true, "ReturnTypeAnnotation": true,
	"TypeParameters": true, "TypeArguments": true, "Implements": true,
	"ComputedType": true, "HoistedDeclarations": true,
}

// walkChildren calls fn on each AST node held by node's fields: directly,
// in slices, or inside helper structs that are not nodes themselves
// (declarators, destructuring elements, catch clauses, switch cases).
func walkChildren(node parser.Node, fn func(parser.Node)) {
	v := reflect.ValueOf(node)
	if v.Kind() != reflect.Ptr || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return
	}
	walkStructFields(v.Elem(), fn)
}

func walkStructFields(v reflect.Value, fn func(parser.Node)) {
	for _, fi := range nodeChildFields(v.Type()) {
		f := v.Field(fi.index)
		if f.Kind() == reflect.Slice {
			for j := 0; j < f.Len(); j++ {
				visitValue(f.Index(j), fi.propertyName, fn)
			}
			continue
		}
		visitValue(f, fi.propertyName, fn)
	}
}

func visitValue(f reflect.Value, propertyName bool, fn func(parser.Node)) {
	if (f.Kind() == reflect.Interface || f.Kind() == reflect.Ptr) && f.IsNil() {
		return
	}
	if n, ok := f.Interface().(parser.Node); ok {
		if isNilASTNode(n) {
			return
		}
		if propertyName {
			if cpn, isComputed := n.(*parser.ComputedPropertyName); isComputed {
				fn(cpn.Expr)
			}
			return
		}
		fn(n)
		return
	}
	if f.Kind() == reflect.Interface {
		f = f.Elem()
	}
	if f.Kind() == reflect.Ptr && f.Elem().Kind() == reflect.Struct && f.Elem().Type().PkgPath() == parserPkgPath {
		walkStructFields(f.Elem(), fn)
	}
}

func nodeChildFields(t reflect.Type) []childField {
	childFieldsMu.RLock()
	fields, ok := childFields[t]
	childFieldsMu.RUnlock()
	if ok {
		return fields
	}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() || sf.Anonymous || skippedChildFields[sf.Name] {
			continue
		}
		ft := sf.Type
		if ft.Kind() == reflect.Slice {
			ft = ft.Elem()
		}
		isStructPtr := ft.Kind() == reflect.Ptr && ft.Elem().Kind() == reflect.Struct && ft.Elem().PkgPath() == parserPkgPath
		if ft.Kind() == reflect.Interface || isStructPtr {
			fields = append(fields, childField{index: i, propertyName: sf.Name == "Key" || sf.Name == "Property"})
		}
	}
	childFieldsMu.Lock()
	childFields[t] = fields
	childFieldsMu.Unlock()
	return fields
}

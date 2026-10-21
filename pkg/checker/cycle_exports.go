package checker

import (
	"sort"

	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// Modules in an import cycle are checked one after another, so the first one
// checked imports from a module whose checker has not run yet. That module
// is checked ahead of its turn up to its declarations (modules.DeclareModule)
// and answers for its exports from them: tsc, which checks a whole program
// at once, resolves such references lazily the same way.

// earlyExport is where an export name of the module being checked comes
// from: a top-level declaration (local, on the type side for interfaces,
// type aliases and namespaces), or another module (source).
type earlyExport struct {
	local    string
	typeSide bool
	source   string
}

// SetDeclaredHook makes Check call hook once the module's declarations are
// in place, before bodies and remaining statements are checked.
func (c *Checker) SetDeclaredHook(hook func()) {
	c.declaredHook = hook
}

// EarlyExport is the type of the module's export name as far as the
// declarations made so far tell. An interface or type alias not declared yet
// is a reference resolved in the module's scope where it is used.
func (c *Checker) EarlyExport(name string) (types.Type, bool) {
	entry, ok := c.earlyExportIndex()[name]
	if !ok {
		for _, star := range c.earlyStarSources() {
			if t, ok := c.earlyExportFrom(star, name); ok {
				return t, true
			}
		}
		return nil, false
	}
	if entry.source != "" {
		return c.earlyExportFrom(entry.source, entry.local)
	}
	scope := c.fileScope
	if scope == nil {
		return nil, false
	}
	if !entry.typeSide {
		if v, _, found := scope.Resolve(entry.local); found && v != nil && v != types.Any {
			if o, ok := v.(*types.ObjectType); ok && (o.Pending || c.declaresClass(entry.local) && len(o.ConstructSigs()) == 0) {
				// A class declared but not yet defined: unresolved, not '{}'.
				return nil, false
			}
			if _, fwd := v.(*types.ForwardReferenceType); fwd {
				return nil, false
			}
			return v, true
		}
	}
	if entry.typeSide {
		c.declarePendingAlias(entry.local)
	}
	if t, found := scope.ResolveType(entry.local); found {
		if o, ok := t.(*types.ObjectType); ok && o.Pending && !entry.typeSide {
			// A class's instance type before the class is defined: an
			// importer wants its constructor.
			return nil, false
		}
		if _, pending := t.(*types.TypeAliasForwardReference); !pending {
			return t, true
		}
	}
	if !entry.typeSide {
		return nil, false
	}
	return &types.GenericTypeAliasForwardReference{AliasName: entry.local, Scope: c.declaredTypeScope}, true
}

// declaredTypeScope looks a type name up in the module's scope for a
// reference another module holds to it. A name still undeclared there is
// any: the importer's own scope must not answer for it.
func (c *Checker) declaredTypeScope(name string) (types.Type, bool) {
	if c.fileScope != nil {
		if t, found := c.fileScope.ResolveType(name); found {
			if _, pending := t.(*types.TypeAliasForwardReference); !pending {
				return t, true
			}
		}
	}
	return types.Any, true
}

// EarlyExportNames lists the module's export names EarlyExport answers for.
func (c *Checker) EarlyExportNames() []string {
	seen := map[string]bool{}
	var names []string
	for name := range c.earlyExportIndex() {
		seen[name] = true
		names = append(names, name)
	}
	for _, star := range c.earlyStarSources() {
		for _, name := range c.earlyExportNamesFrom(star) {
			if !seen[name] && name != "default" {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// earlyExportIndex maps the module's export names to where they come from.
func (c *Checker) earlyExportIndex() map[string]earlyExport {
	if c.earlyExports != nil || c.program == nil {
		return c.earlyExports
	}
	index := map[string]earlyExport{}
	for _, stmt := range c.program.Statements {
		switch s := stmt.(type) {
		case *parser.ExportNamedDeclaration:
			if s.Declaration != nil {
				for _, d := range earlyDeclarationExports(s.Declaration) {
					index[d.local] = d
				}
				continue
			}
			for _, spec := range s.Specifiers {
				named, ok := spec.(*parser.ExportNamedSpecifier)
				if !ok || named.Local == nil {
					continue
				}
				local := getExportSpecName(named.Local)
				exported := getExportSpecName(named.Exported)
				entry := earlyExport{local: local, typeSide: s.IsTypeOnly || named.IsTypeOnly}
				if s.Source != nil {
					entry.source = s.Source.Value
				}
				index[exported] = entry
			}
		case *parser.ExportDefaultDeclaration:
			if id, ok := s.Declaration.(*parser.Identifier); ok {
				index["default"] = earlyExport{local: id.Value}
			}
		case *parser.ExportAllDeclaration:
			if s.Exported != nil && s.Source != nil {
				index[getExportSpecName(s.Exported)] = earlyExport{source: s.Source.Value, local: "*"}
			}
		}
	}
	for _, decl := range c.implicitExports() {
		for _, d := range earlyDeclarationExports(decl) {
			if _, has := index[d.local]; !has {
				index[d.local] = d
			}
		}
	}
	c.earlyExports = index
	return index
}

// implicitExports are the top-level declarations a declaration file exports
// without `export`: tsc treats a .d.ts module with no export list, `export *`
// or export assignment as ambient, and exports every declaration in it
// (fastify's hooks.d.ts declares HookHandlerDoneFunction bare, and
// fastify.d.ts imports it).
func (c *Checker) implicitExports() []parser.Statement {
	if c.program == nil || !c.IsModuleMode() || !c.inDeclarationFile() {
		return nil
	}
	var decls []parser.Statement
	for _, stmt := range c.program.Statements {
		switch s := stmt.(type) {
		case *parser.ExportNamedDeclaration:
			if s.Declaration == nil {
				return nil
			}
		case *parser.ExportAllDeclaration, *parser.ExportAssignment:
			return nil
		case *parser.ExportDefaultDeclaration:
			if !s.IsDeclaration {
				return nil
			}
		case *parser.InterfaceDeclaration, *parser.TypeAliasStatement, *parser.ClassDeclaration,
			*parser.FunctionSignature, *parser.LetStatement, *parser.ConstStatement, *parser.VarStatement,
			*parser.DeclarationGroup:
			decls = append(decls, stmt)
		case *parser.NamespaceDeclaration:
			if !s.AmbientModule {
				decls = append(decls, stmt)
			}
		case *parser.ExpressionStatement:
			switch s.Expression.(type) {
			case *parser.FunctionLiteral, *parser.ClassExpression, *parser.EnumDeclaration:
				decls = append(decls, stmt)
			}
		}
	}
	return decls
}

// earlyStarSources are the modules `export * from` re-exports.
func (c *Checker) earlyStarSources() []string {
	if c.program == nil {
		return nil
	}
	var sources []string
	for _, stmt := range c.program.Statements {
		if s, ok := stmt.(*parser.ExportAllDeclaration); ok && s.Exported == nil && s.Source != nil {
			sources = append(sources, s.Source.Value)
		}
	}
	return sources
}

// earlyDeclarationExports are the names an exported declaration binds.
func earlyDeclarationExports(decl parser.Statement) []earlyExport {
	switch d := decl.(type) {
	case *parser.InterfaceDeclaration:
		if d.Name != nil {
			return []earlyExport{{local: d.Name.Value, typeSide: true}}
		}
	case *parser.TypeAliasStatement:
		if d.Name != nil {
			return []earlyExport{{local: d.Name.Value, typeSide: true}}
		}
	case *parser.NamespaceDeclaration:
		if d.Name != nil && !d.AmbientModule {
			return []earlyExport{{local: d.Name.Value, typeSide: true}}
		}
	case *parser.ClassDeclaration:
		if d.Name != nil {
			return []earlyExport{{local: d.Name.Value}}
		}
	case *parser.FunctionSignature:
		if d.Name != nil {
			return []earlyExport{{local: d.Name.Value}}
		}
	case *parser.ExpressionStatement:
		switch e := d.Expression.(type) {
		case *parser.FunctionLiteral:
			if e.Name != nil {
				return []earlyExport{{local: e.Name.Value}}
			}
		case *parser.ClassExpression:
			if e.Name != nil {
				return []earlyExport{{local: e.Name.Value}}
			}
		case *parser.EnumDeclaration:
			if e.Name != nil {
				return []earlyExport{{local: e.Name.Value}}
			}
		}
	default:
		var out []earlyExport
		for _, name := range parser.DeclaredNames(decl) {
			out = append(out, earlyExport{local: name})
		}
		return out
	}
	return nil
}

// earlyExportFrom is export name of the module specifier names, from its
// exports or, while it is being checked, its early exports.
func (c *Checker) earlyExportFrom(specifier, name string) (types.Type, bool) {
	if c.moduleEnv == nil || c.earlyLookups[specifier+"\x00"+name] {
		return nil, false
	}
	if c.earlyLookups == nil {
		c.earlyLookups = map[string]bool{}
	}
	key := specifier + "\x00" + name
	c.earlyLookups[key] = true
	defer delete(c.earlyLookups, key)
	rec := c.moduleEnv.sourceRecord(specifier)
	if rec == nil {
		return nil, false
	}
	if name == "*" {
		exports := map[string]types.Type{}
		if rec.Exports != nil {
			exports = rec.Exports
		} else if rec.EarlyExport != nil {
			for _, n := range rec.EarlyExportNames() {
				if t, ok := rec.EarlyExport(n); ok {
					exports[n] = t
				}
			}
		}
		return c.moduleEnv.createNamespaceType(exports), true
	}
	if rec.Exports != nil {
		t, ok := rec.Exports[name]
		return t, ok
	}
	if c.moduleEnv.declareEarly(rec) {
		return rec.EarlyExport(name)
	}
	return nil, false
}

func (c *Checker) earlyExportNamesFrom(specifier string) []string {
	if c.moduleEnv == nil || c.earlyLookups[specifier+"\x00*names"] {
		return nil
	}
	if c.earlyLookups == nil {
		c.earlyLookups = map[string]bool{}
	}
	key := specifier + "\x00*names"
	c.earlyLookups[key] = true
	defer delete(c.earlyLookups, key)
	rec := c.moduleEnv.sourceRecord(specifier)
	if rec == nil {
		return nil
	}
	if rec.Exports != nil {
		names := make([]string, 0, len(rec.Exports))
		for n := range rec.Exports {
			names = append(names, n)
		}
		sort.Strings(names)
		return names
	}
	if c.moduleEnv.declareEarly(rec) {
		return rec.EarlyExportNames()
	}
	return nil
}

// sourceRecord is the record of the module specifier names from this one,
// loading it if needed.
func (me *ModuleEnvironment) sourceRecord(specifier string) *modules.ModuleRecord {
	if me.ModuleLoader == nil {
		return nil
	}
	if rec := me.ModuleLoader.GetModule(specifier, me.ModulePath); rec != nil {
		return rec
	}
	loaded, err := me.ModuleLoader.LoadModule(specifier, me.ModulePath)
	if err != nil {
		return nil
	}
	rec, _ := loaded.(*modules.ModuleRecord)
	return rec
}

// declareEarly makes rec, a module without exports yet, answer through
// EarlyExport, checking it ahead of its turn if no one is checking it yet.
func (me *ModuleEnvironment) declareEarly(rec *modules.ModuleRecord) bool {
	if rec.EarlyExport == nil {
		if d, ok := me.ModuleLoader.(interface{ DeclareModule(*modules.ModuleRecord) }); ok {
			d.DeclareModule(rec)
		}
	}
	return rec.EarlyExport != nil
}

// earlyImport resolves binding from rec, a module still being checked.
func (me *ModuleEnvironment) earlyImport(rec *modules.ModuleRecord, binding *ImportBinding) (types.Type, bool) {
	if !me.declareEarly(rec) {
		return nil, false
	}
	binding.Early = true
	if binding.ImportType == ImportNamespace {
		exports := map[string]types.Type{}
		for _, n := range rec.EarlyExportNames() {
			if t, ok := rec.EarlyExport(n); ok {
				exports[n] = t
			}
		}
		return me.createNamespaceType(exports), true
	}
	return rec.EarlyExport(binding.SourceName)
}

// refreshEarlyImports resolves again the imports resolved from a module that
// was still being checked, now that the modules this one waited for are done.
func (c *Checker) refreshEarlyImports() {
	if c.moduleEnv == nil {
		return
	}
	names := make([]string, 0)
	for local, binding := range c.moduleEnv.ImportedNames {
		if binding.Early {
			names = append(names, local)
		}
	}
	sort.Strings(names)
	for _, local := range names {
		binding := c.moduleEnv.ImportedNames[local]
		binding.Early = false
		binding.ResolvedType = types.Any
		// What it was bound to (any, while unresolved) gives way.
		if c.fileScope != nil {
			delete(c.fileScope.typeAliases, local)
		}
		c.bindImport(local, binding.ImportType, binding.IsTypeOnly)
	}
	// Interfaces extending what those modules had not declared yet.
	c.resolveDeferredHeritages()
}

// declaresClass reports whether the program declares a class name at the top
// level, exported or not.
func (c *Checker) declaresClass(name string) bool {
	if c.program == nil {
		return false
	}
	for _, stmt := range c.program.Statements {
		if e, ok := stmt.(*parser.ExportNamedDeclaration); ok && e.Declaration != nil {
			stmt = e.Declaration
		}
		switch d := stmt.(type) {
		case *parser.ClassDeclaration:
			if d.Name != nil && d.Name.Value == name {
				return true
			}
		case *parser.ExpressionStatement:
			if ce, ok := d.Expression.(*parser.ClassExpression); ok && ce.Name != nil && ce.Name.Value == name {
				return true
			}
		}
	}
	return false
}

// declarePendingAlias declares the type alias name, not reached yet in Pass
// 1, for a module importing it: a reference to it left unresolved would be
// instantiated, and its conditional and indexed members evaluated, before
// it means anything (RootConfig<AnyRootTypes> in tRPC). An alias naming what
// this module has not bound yet (a later import) reports errors; it is then
// left pending, and the importer gets a reference after all.
func (c *Checker) declarePendingAlias(name string) {
	node, ok := c.pendingAliases[name]
	env := c.pendingAliasEnv
	if !ok || env == nil || c.resolvingTypeAliases[name] {
		return
	}
	saved, had := env.typeAliases[name]
	if c.speculate(func() { c.resolvePendingAlias(name) }) {
		// Declared ahead of what it names (a generic interface further up
		// that Pass 1 has not reached), it would keep a reference to it
		// that source order would have resolved.
		if t, ok := env.typeAliases[name]; !ok || !holdsForwardReference(t, 0) {
			return
		}
	}
	if had {
		env.typeAliases[name] = saved
	} else {
		delete(env.typeAliases, name)
	}
	c.pendingAliases[name] = node
}

// holdsForwardReference reports whether t, an alias's type, still refers to
// a type not declared yet (looking a few levels into the type).
func holdsForwardReference(t types.Type, depth int) bool {
	if depth > 4 || t == nil {
		return false
	}
	switch tt := t.(type) {
	case *types.GenericTypeAliasForwardReference, *types.TypeAliasForwardReference, *types.ForwardReferenceType, *types.ParameterizedForwardReferenceType,
		*types.TypeofType:
		// A typeof of a value declared later is resolved in Pass 2.
		return true
	case *types.GenericType:
		return holdsForwardReference(tt.Body, depth+1)
	case *types.UnionType:
		for _, m := range tt.Types {
			if holdsForwardReference(m, depth+1) {
				return true
			}
		}
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if holdsForwardReference(m, depth+1) {
				return true
			}
		}
	case *types.ArrayType:
		return holdsForwardReference(tt.ElementType, depth+1)
	case *types.InstantiatedType:
		for _, a := range tt.TypeArguments {
			if holdsForwardReference(a, depth+1) {
				return true
			}
		}
	case *types.ObjectType:
		if tt.Pending {
			return true
		}
		for _, a := range tt.GenericArgs {
			if holdsForwardReference(a, depth+1) {
				return true
			}
		}
	case *types.ConditionalType:
		return holdsForwardReference(tt.CheckType, depth+1) || holdsForwardReference(tt.ExtendsType, depth+1) ||
			holdsForwardReference(tt.TrueType, depth+1) || holdsForwardReference(tt.FalseType, depth+1)
	case *types.IndexedAccessType:
		return holdsForwardReference(tt.ObjectType, depth+1) || holdsForwardReference(tt.IndexType, depth+1)
	}
	return false
}

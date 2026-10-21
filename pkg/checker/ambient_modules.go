package checker

import (
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// An ambient module declaration, `declare module "fs" { ... }` in a script
// file (a declaration file without imports or exports, such as @types/node's),
// declares the module "fs": importing "fs" anywhere in the program gets its
// members, ahead of resolving "fs" to a file, as tsc does. Declarations of the
// same module in several files merge (@types/node declares "buffer" in two).
//
// The body is checked like a namespace's. Its own imports bind in the body,
// and `export { ... }`, `export * from` and `export =` in it describe the
// module, not the file the declaration is in.

// ambientModule is the module one or more ambient module declarations
// declare.
type ambientModule struct {
	ns           *types.NamespaceType
	exportEquals types.Type
	// exportEqualsValue is the value export= names when it names both a
	// namespace and a value (`namespace path {...} const path:
	// path.PlatformPath; export = path`).
	exportEqualsValue types.Type
}

// ambientModuleFor returns the module name declares, creating it on first
// declaration.
func (g *GlobalDeclarations) ambientModuleFor(name string) *ambientModule {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ambient == nil {
		g.ambient = map[string]*ambientModule{}
	}
	m := g.ambient[name]
	if m == nil {
		ns := types.NewNamespaceType(name)
		ns.Declare = true
		ns.Instantiated = true
		m = &ambientModule{ns: ns}
		g.ambient[name] = m
	}
	return m
}

// AmbientExports is the exports of the module an ambient module declaration
// named specifier declares, or nil if none does. It is the module loader's
// ambient lookup (see modules.ModuleLoader.SetAmbientModules).
func (g *GlobalDeclarations) AmbientExports(specifier string) map[string]types.Type {
	g.mu.Lock()
	m := g.ambient[specifier]
	g.mu.Unlock()
	if m == nil {
		return nil
	}
	return m.exports()
}

// exports lists the module's exports the way a checked module's record does:
// one type per name, the value where there is one (a class's constructor,
// whose instance type importedTypeAlias recovers), and export= with its
// members.
func (m *ambientModule) exports() map[string]types.Type {
	exports := make(map[string]types.Type, len(m.ns.TypeMembers)+len(m.ns.ValueShape.Props()))
	addNamespace := func(ns *types.NamespaceType) {
		for name, t := range ns.TypeMembers {
			if _, done := exports[name]; done {
				continue
			}
			if v, ok := ns.ValueShape.Props()[name]; ok {
				if inner, isNs := t.(*types.NamespaceType); !isNs || inner.ValueShape != v {
					t = v
				}
			}
			exports[name] = t
		}
		for name, v := range ns.ValueShape.Props() {
			if _, done := exports[name]; !done {
				exports[name] = v
			}
		}
	}
	addNamespace(m.ns)
	if m.exportEquals != nil {
		entity := m.exportEquals
		if m.exportEqualsValue != nil {
			entity = m.exportEqualsValue
		}
		exports[exportEqualsName] = entity
		if _, has := exports["default"]; !has {
			exports["default"] = entity
		}
		if obj, ok := m.exportEqualsValue.(*types.ObjectType); ok {
			for _, name := range obj.PropertyNames() {
				if _, done := exports[name]; !done {
					exports[name] = obj.Props()[name]
				}
			}
		}
		if ns, ok := m.exportEquals.(*types.NamespaceType); ok {
			addNamespace(ns)
		}
	}
	return exports
}

// currentFilePath is the path of the file being checked, for resolving the
// imports in it.
func (c *Checker) currentFilePath() string {
	if c.moduleEnv != nil && c.moduleEnv.ModulePath != "" {
		return c.moduleEnv.ModulePath
	}
	if c.source != nil {
		return c.source.Path
	}
	return ""
}

// moduleExports loads the module specifier names from the file being
// checked and returns its exports, or nil.
func (c *Checker) moduleExports(specifier string) map[string]types.Type {
	if c.moduleLoader == nil {
		return nil
	}
	loaded, err := c.moduleLoader.LoadModule(specifier, c.currentFilePath())
	if err != nil {
		return nil
	}
	rec, ok := loaded.(*modules.ModuleRecord)
	if !ok || rec == nil {
		return nil
	}
	return rec.Exports
}

// bindAmbientModuleImports binds the names an ambient module body imports in
// env, before the body's declarations are resolved. A module that cannot be
// found yields any: in a declaration file the import is usually of another
// ambient module that a file not checked yet declares.
func (c *Checker) bindAmbientModuleImports(body *parser.BlockStatement, env *Environment) {
	for _, stmt := range body.Statements {
		imp, ok := stmt.(*parser.ImportDeclaration)
		if !ok || imp.Source == nil {
			continue
		}
		exports := c.moduleExports(imp.Source.Value)
		lookup := func(name string) types.Type {
			if t, ok := exports[name]; ok && t != nil {
				return t
			}
			return types.Any
		}
		for _, spec := range imp.Specifiers {
			switch s := spec.(type) {
			case *parser.ImportNamedSpecifier:
				if s.Local != nil && s.Imported != nil {
					bindImported(env, s.Local.Value, lookup(s.Imported.Value))
				}
			case *parser.ImportDefaultSpecifier:
				if s.Local != nil {
					bindImported(env, s.Local.Value, lookup("default"))
				}
			case *parser.ImportNamespaceSpecifier:
				if s.Local == nil {
					continue
				}
				if exports == nil {
					bindImported(env, s.Local.Value, types.Any)
					continue
				}
				ns := moduleNamespace(s.Local.Value, exports)
				env.DefineTypeAlias(s.Local.Value, ns)
				env.Define(s.Local.Value, ns.ValueShape, false)
			}
		}
	}
}

// bindImported binds an imported export as both a type and a value.
func bindImported(env *Environment, name string, t types.Type) {
	env.DefineTypeAlias(name, importedTypeAlias(t))
	env.Define(name, importedValue(t), false)
}

// setNamespaceExport makes t the member name of ns, on both sides.
func setNamespaceExport(ns *types.NamespaceType, name string, t types.Type) {
	ns.TypeMembers[name] = importedTypeAlias(t)
	ns.ValueShape.SetProperty(name, importedValue(t))
}

// checkAmbientModuleStatement handles the statements of an ambient module
// body that describe the module itself. It reports whether stmt was one.
func (c *Checker) checkAmbientModuleStatement(stmt parser.Statement, m *ambientModule) bool {
	switch n := stmt.(type) {
	case *parser.ImportDeclaration:
		return true // bound before the body (bindAmbientModuleImports)

	case *parser.ExportNamedDeclaration:
		if n.Declaration != nil {
			return false
		}
		var exports map[string]types.Type
		if n.Source != nil {
			exports = c.moduleExports(n.Source.Value)
		}
		for _, spec := range n.Specifiers {
			s, ok := spec.(*parser.ExportNamedSpecifier)
			if !ok {
				continue
			}
			local, exported := exportSpecifierName(s.Local), exportSpecifierName(s.Exported)
			if exported == "" {
				exported = local
			}
			if local == "" {
				continue
			}
			if n.Source != nil {
				if t, ok := exports[local]; ok && t != nil {
					setNamespaceExport(m.ns, exported, t)
				}
				continue
			}
			// A name another declaration of the module declares is
			// already a member; one not declared yet is left to it.
			if t, found := c.env.ResolveType(local); found && t != nil {
				m.ns.TypeMembers[exported] = t
			}
			if v, _, found := c.env.Resolve(local); found && v != nil {
				m.ns.ValueShape.SetProperty(exported, v)
			}
		}
		return true

	case *parser.ExportAllDeclaration:
		if n.Source == nil {
			return true
		}
		exports := c.moduleExports(n.Source.Value)
		if exports == nil {
			return true
		}
		if n.Exported != nil {
			if name := exportSpecifierName(n.Exported); name != "" {
				ns := moduleNamespace(name, exports)
				m.ns.TypeMembers[name] = ns
				m.ns.ValueShape.SetProperty(name, ns.ValueShape)
			}
			return true
		}
		for name, t := range exports {
			if name == "default" || name == exportEqualsName {
				continue
			}
			if _, own := m.ns.TypeMembers[name]; own {
				continue
			}
			if _, own := m.ns.ValueShape.Props()[name]; own {
				continue
			}
			setNamespaceExport(m.ns, name, t)
		}
		return true

	case *parser.ExportAssignment:
		if n.Expression == nil {
			return true
		}
		if id, ok := n.Expression.(*parser.Identifier); ok {
			if c.exportEqualsRequireAlias(id.Value, m) {
				return true
			}
			var entity types.Type
			if t, found := c.env.ResolveType(id.Value); found {
				entity = t
			}
			v, _, hasValue := c.env.Resolve(id.Value)
			if merged := c.env.ResolveMergedNamespace(id.Value); merged != nil && hasValue {
				// A class or function merged with a namespace: export= is
				// the value, and the namespace's members are exported.
				m.exportEquals, m.exportEqualsValue = merged, v
				return true
			}
			if ns, isNs := entity.(*types.NamespaceType); !isNs {
				if hasValue {
					entity = v
				}
			} else if hasValue && v != nil && v != ns.ValueShape {
				// A value merged with the namespace: export= is both.
				m.exportEqualsValue = v
			}
			if entity == nil {
				c.addCannotFindNameError(id, c.env, id.Value)
				return true
			}
			m.exportEquals = entity
			return true
		}
		c.visit(n.Expression)
		m.exportEquals = n.Expression.GetComputedType()
		return true
	}
	return false
}

// exportSpecifierName is the text of an export specifier's name, an
// identifier or a string literal.
func exportSpecifierName(e parser.Expression) string {
	switch n := e.(type) {
	case *parser.Identifier:
		return n.Value
	case *parser.StringLiteral:
		return n.Value
	}
	return ""
}

// exportEqualsRequireAlias handles `export = x` where x is
// `import x = require("m")` (@types/node's node:stream re-exporting stream):
// the alias was bound before m was complete, so m's exports are read again
// now. It reports whether name was such an alias.
func (c *Checker) exportEqualsRequireAlias(name string, m *ambientModule) bool {
	spec := c.env.ResolveRequireAlias(name)
	if spec == "" {
		return false
	}
	exports := c.moduleExports(spec)
	value, ok := exports[exportEqualsName]
	if !ok {
		return false
	}
	members := make(map[string]types.Type, len(exports))
	for member, t := range exports {
		if member != exportEqualsName && member != "default" {
			members[member] = t
		}
	}
	m.exportEquals, m.exportEqualsValue = moduleNamespace(name, members), value
	return true
}

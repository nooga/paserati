package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// exportEqualsName is the export a module's `export = X` is recorded under.
const exportEqualsName = "export="

// checkImportEqualsDeclaration binds `import x = require("m")` and
// `import x = N.M`, returning the bound type and value sides (nil when the
// name has none) so a namespace body can publish an exported alias.
func (c *Checker) checkImportEqualsDeclaration(node *parser.ImportEqualsDeclaration) (types.Type, types.Type) {
	if node == nil || node.Name == nil {
		return nil, nil
	}
	name := node.Name.Value
	if node.Require != nil {
		return c.bindRequireImport(node, name)
	}

	// An entity alias names both sides of the entity: its type meaning and,
	// when it has one, its value.
	var typeSide, valueSide types.Type
	switch e := node.Entity.(type) {
	case *parser.Identifier:
		if t, found := c.env.ResolveType(e.Value); found {
			typeSide = t
		}
		if t, _, found := c.env.Resolve(e.Value); found {
			valueSide = t
		}
	case *parser.MemberExpression:
		typeSide = c.resolveQualifiedTypeQuietly(e)
		valueSide = c.resolveQualifiedValueQuietly(e)
	}
	if typeSide == nil && valueSide == nil {
		c.addErrorWithCode(node.Entity, errors.TS2304, fmt.Sprintf("Cannot find name '%s'.", node.Entity.String()))
		return nil, nil
	}
	if typeSide != nil {
		c.env.DefineTypeAlias(name, typeSide)
	}
	if valueSide != nil && !node.IsTypeOnly {
		if !c.env.Define(name, valueSide, true) {
			c.env.Update(name, valueSide)
		}
	}
	if node.IsExported && c.IsModuleMode() && c.namespaceBodyDepth == 0 {
		exported := valueSide
		if ns, ok := typeSide.(*types.NamespaceType); ok || exported == nil {
			exported = typeSide
			_ = ns
		}
		c.moduleEnv.DefineExport(name, name, exported, nil)
	}
	return typeSide, valueSide
}

// bindRequireImport binds `import x = require("m")`: the module's `export =`
// entity when it has one, else its namespace.
func (c *Checker) bindRequireImport(node *parser.ImportEqualsDeclaration, name string) (types.Type, types.Type) {
	spec := node.Require.Value
	c.env.recordRequireAlias(name, spec)
	var rec *modules.ModuleRecord
	if c.moduleLoader != nil {
		from := ""
		if c.moduleEnv != nil {
			from = c.moduleEnv.ModulePath
		} else if c.source != nil {
			from = c.source.Path
		}
		if loaded, err := c.moduleLoader.LoadModule(spec, from); err == nil {
			rec, _ = loaded.(*modules.ModuleRecord)
		}
	}
	if rec == nil {
		c.addErrorWithCode(node.Require, errors.TS2307, fmt.Sprintf("Cannot find module '%s' or its corresponding type declarations.", spec))
		c.env.DefineTypeAlias(name, types.Any)
		if !node.IsTypeOnly {
			c.env.Define(name, types.Any, true)
		}
		return types.Any, types.Any
	}
	if !c.IsModuleMode() {
		ns := moduleNamespace(name, rec.Exports)
		c.env.DefineTypeAlias(name, ns)
		if !node.IsTypeOnly {
			c.env.Define(name, ns.ValueShape, true)
		}
		return ns, ns.ValueShape
	}
	if _, ok := rec.Exports[exportEqualsName]; ok {
		c.processImportBinding(name, spec, exportEqualsName, ImportNamed, node.IsTypeOnly)
		if t, _ := c.env.ResolveType(name); !isNamespaceType(t) {
			// export= of a class or function merged with a namespace: the
			// module's other exports are that namespace's members (x.T).
			members := make(map[string]types.Type, len(rec.Exports))
			for member, t := range rec.Exports {
				if member != exportEqualsName && member != "default" {
					members[member] = t
				}
			}
			if len(members) > 0 {
				c.env.recordMergedNamespace(name, moduleNamespace(name, members))
			}
		}
	} else {
		c.processImportBinding(name, spec, "*", ImportNamespace, node.IsTypeOnly)
	}
	typeSide, _ := c.env.ResolveType(name)
	valueSide, _, _ := c.env.Resolve(name)
	return typeSide, valueSide
}

// resolveQualifiedTypeQuietly resolves N.M to the type it names, or nil,
// without reporting.
func (c *Checker) resolveQualifiedTypeQuietly(e *parser.MemberExpression) types.Type {
	prop, ok := e.Property.(*parser.Identifier)
	if !ok {
		return nil
	}
	var container types.Type
	switch obj := e.Object.(type) {
	case *parser.Identifier:
		container, _ = c.env.ResolveType(obj.Value)
		if _, isNs := container.(*types.NamespaceType); !isNs {
			if ns := c.env.ResolveMergedNamespace(obj.Value); ns != nil {
				container = ns
			}
		}
	case *parser.MemberExpression:
		container = c.resolveQualifiedTypeQuietly(obj)
	}
	if ns, ok := container.(*types.NamespaceType); ok {
		if m := ns.LookupTypeMember(prop.Value); m != nil {
			return m
		}
	}
	return nil
}

// resolveQualifiedValueQuietly resolves the value N.M (globalThis.atob), or
// nil, without reporting.
func (c *Checker) resolveQualifiedValueQuietly(e *parser.MemberExpression) types.Type {
	prop, ok := e.Property.(*parser.Identifier)
	if !ok {
		return nil
	}
	var container types.Type
	switch obj := e.Object.(type) {
	case *parser.Identifier:
		if obj.Value == "globalThis" {
			if t, _, found := c.env.Resolve(prop.Value); found {
				return t
			}
			return nil
		}
		container, _, _ = c.env.Resolve(obj.Value)
	case *parser.MemberExpression:
		container = c.resolveQualifiedValueQuietly(obj)
	}
	if container == nil {
		return nil
	}
	if t := c.rel.GetPropertyType(container, prop.Value, false); t != nil && t != types.Any {
		return t
	}
	return nil
}

// checkExportAssignment records `export = X` as the module's whole export:
// under "export=" (for `import x = require`) and "default" (an ES default
// import of a CommonJS module, as with esModuleInterop). The members of a
// namespace X are also exported by name, so `import { Router } from
// "express"` works.
func (c *Checker) checkExportAssignment(node *parser.ExportAssignment) {
	if node == nil || node.Expression == nil {
		return
	}
	var entity types.Type
	var ns *types.NamespaceType
	if id, ok := node.Expression.(*parser.Identifier); ok {
		if t, found := c.env.ResolveType(id.Value); found {
			ns, _ = t.(*types.NamespaceType)
			entity = t
		}
		if ns == nil {
			if t, _, found := c.env.Resolve(id.Value); found {
				entity = t
			}
			// A class or function merged with a namespace exports the
			// namespace's members too.
			ns = c.env.ResolveMergedNamespace(id.Value)
		}
		if entity == nil {
			c.addCannotFindNameError(id, c.env, id.Value)
			return
		}
	} else {
		c.visit(node.Expression)
		entity = node.Expression.GetComputedType()
	}
	if entity == nil || !c.IsModuleMode() {
		return
	}
	c.moduleEnv.DefineExport(exportEqualsName, exportEqualsName, entity, nil)
	if _, has := c.moduleEnv.GetExportedType("default"); !has {
		c.moduleEnv.DefineExport("default", "default", entity, nil)
	}
	if ns == nil {
		return
	}
	for name, t := range ns.TypeMembers {
		if v, ok := ns.ValueShape.Props()[name]; ok {
			// A nested namespace stays one (React.JSX.Element) unless its
			// value is something more, such as a merged function.
			if inner, isNs := t.(*types.NamespaceType); !isNs || inner.ValueShape != v {
				t = v
			}
		}
		c.moduleEnv.DefineExport(name, name, t, nil)
	}
	for name, v := range ns.ValueShape.Props() {
		if _, done := ns.TypeMembers[name]; !done {
			c.moduleEnv.DefineExport(name, name, v, nil)
		}
	}
}

func isNamespaceType(t types.Type) bool {
	_, ok := t.(*types.NamespaceType)
	return ok
}

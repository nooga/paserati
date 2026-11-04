package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
)

// A name imported with `import type` (or `import { type X }`, `import type X
// = require()`) has no value: using it in an expression is TS1361, as tsc's
// resolveNameHelper reports for an alias whose only declaration on the value
// side is type-only. A plain import of a name its module exported with
// `export type` (directly or through re-exports) is TS1362; one its module
// re-exported from an `import type` is TS1361 again. The use sites tsc
// accepts (isValidTypeOnlyAliasUseSite) are type positions and ambient code,
// which do not visit the name as a value, `export default X`, which
// re-exports the alias, and the computed keys of type members.
//
// FIXME: a namespace import's members (`ns.A` of an `export type { A }`)
// and `export type * as ns` are not tracked.

// checkTypeOnlyImportUse reports a value use of a type-only import.
func (c *Checker) checkTypeOnlyImportUse(node *parser.Identifier) {
	if c.moduleEnv == nil || c.typeOnlyUseAllowed || c.inAmbientNamespace {
		return
	}
	binding := c.moduleEnv.ImportedNames[node.Value]
	if binding == nil || !binding.IsTypeOnly && binding.TypeOnlyVia == "" {
		return
	}
	if env := c.definingEnv(node.Value); env != c.moduleEnv.Environment && env != c.fileScope || c.declaresLocalValue(node.Value) {
		return // a local declaration shadows the import, or merges with it
	}
	if !binding.IsTypeOnly && binding.TypeOnlyVia == "export" {
		c.addErrorWithCode(node, errors.TS1362, fmt.Sprintf("'%s' cannot be used as a value because it was exported using 'export type'.", node.Value))
		return
	}
	c.addErrorWithCode(node, errors.TS1361, fmt.Sprintf("'%s' cannot be used as a value because it was imported using 'import type'.", node.Value))
}

// declaresLocalValue reports whether the file declares a value named name
// at the top level, which merges with an import of a type of that name.
func (c *Checker) declaresLocalValue(name string) bool {
	if c.varOnlyDecls[name] != nil {
		return true
	}
	if m := c.blockScoped[c.fileScope]; m != nil && m[name] != nil {
		return true
	}
	if m := c.blockScoped[c.moduleEnv.Environment]; m != nil && m[name] != nil {
		return true
	}
	return false
}

// sourceExportTypeOnly is how the export name of the module spec (as
// imported here) is type-only, "" if it is not.
func (c *Checker) sourceExportTypeOnly(spec, name string) string {
	if c.moduleEnv == nil || c.moduleEnv.ModuleLoader == nil || name == "*" {
		return ""
	}
	rec := c.moduleEnv.ModuleLoader.GetModule(spec, c.moduleEnv.ModulePath)
	if rec == nil {
		if _, err := c.moduleEnv.ModuleLoader.LoadModule(spec, c.moduleEnv.ModulePath); err != nil {
			return ""
		}
		rec = c.moduleEnv.ModuleLoader.GetModule(spec, c.moduleEnv.ModulePath)
	}
	if rec == nil {
		return ""
	}
	return rec.TypeOnlyExports[name]
}

// TypeOnlyExports implements modules.TypeOnlyExportsProvider: the module's
// exports that only name a type because of a type-only declaration.
func (c *Checker) TypeOnlyExports() map[string]string {
	if c.moduleEnv == nil {
		return nil
	}
	var out map[string]string
	set := func(name, via string) {
		if via == "" {
			return
		}
		if out == nil {
			out = map[string]string{}
		}
		out[name] = via
	}
	viaImport := func(local string) string {
		b := c.moduleEnv.ImportedNames[local]
		if b == nil || c.declaresLocalValue(local) {
			return ""
		}
		if b.IsTypeOnly {
			return "import"
		}
		return b.TypeOnlyVia
	}
	for name, b := range c.moduleEnv.ExportedNames {
		switch {
		case b.IsTypeOnly:
			set(name, "export")
		case b.IsReExport:
			set(name, c.sourceExportTypeOnly(b.SourceModule, b.LocalName))
		default:
			set(name, viaImport(b.LocalName))
		}
	}
	if c.defaultExportName != "" {
		set("default", viaImport(c.defaultExportName))
	}
	return out
}

var _ modules.TypeOnlyExportsProvider = (*Checker)(nil)

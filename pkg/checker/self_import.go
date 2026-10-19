package checker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// A module may import itself by its own file name
// (`import { "<X>" as x } from "./thisFile"`). Without a module loader that
// knows the file, such imports are bound here from the file's own exports.

// isSelfModule reports whether a module specifier names the checked file.
func (c *Checker) isSelfModule(spec string) bool {
	if c.sourceFileName == "" || !strings.HasPrefix(spec, "./") {
		return false
	}
	name := strings.TrimPrefix(spec, "./")
	base := strings.TrimSuffix(c.sourceFileName, filepath.Ext(c.sourceFileName))
	return name == c.sourceFileName || name == base || strings.TrimSuffix(name, filepath.Ext(name)) == base
}

// selfExport is what one of the file's export names refers to: a local
// declaration, another export name of the file (a re-export from itself),
// or the file's namespace.
type selfExport struct {
	local     string
	reexport  string
	namespace bool
}

// selfExports maps the file's export names syntactically.
func (c *Checker) selfExports() map[string]selfExport {
	exports := map[string]selfExport{}
	if c.program == nil {
		return exports
	}
	for _, stmt := range c.program.Statements {
		switch s := stmt.(type) {
		case *parser.ExportNamedDeclaration:
			if s.Declaration != nil {
				for _, name := range exportedDeclarationNames(s.Declaration) {
					exports[name] = selfExport{local: name}
				}
				continue
			}
			self := s.Source != nil && c.isSelfModule(s.Source.Value)
			if s.Source != nil && !self {
				continue
			}
			for _, spec := range s.Specifiers {
				es, ok := spec.(*parser.ExportNamedSpecifier)
				if !ok {
					continue
				}
				local, exported := getExportSpecName(es.Local), getExportSpecName(es.Exported)
				if es.Exported == nil {
					exported = local
				}
				if self {
					exports[exported] = selfExport{reexport: local}
				} else {
					exports[exported] = selfExport{local: local}
				}
			}
		case *parser.ExportAllDeclaration:
			if s.Exported != nil && s.Source != nil && c.isSelfModule(s.Source.Value) {
				exports[getExportSpecName(s.Exported)] = selfExport{namespace: true}
			}
		}
	}
	return exports
}

// exportedDeclarationNames are the names an exported declaration binds.
func exportedDeclarationNames(decl parser.Statement) []string {
	switch d := decl.(type) {
	case *parser.TypeAliasStatement:
		if d.Name != nil {
			return []string{d.Name.Value}
		}
	case *parser.InterfaceDeclaration:
		if d.Name != nil {
			return []string{d.Name.Value}
		}
	case *parser.LetStatement, *parser.ConstStatement, *parser.VarStatement:
		return parser.DeclaredNames(decl)
	}
	return nil
}

// resolveSelfExport follows re-exports to a local name or the namespace.
func resolveSelfExport(exports map[string]selfExport, name string) (selfExport, bool) {
	for i := 0; i < 16; i++ {
		e, ok := exports[name]
		if !ok {
			return selfExport{}, false
		}
		if e.reexport == "" {
			return e, true
		}
		name = e.reexport
	}
	return selfExport{}, false
}

// bindSelfImport binds an import from the checked file itself. Type names
// resolve to the local declarations; values are any (they are checked
// where they are declared).
func (c *Checker) bindSelfImport(node *parser.ImportDeclaration) {
	exports := c.selfExports()
	for _, spec := range node.Specifiers {
		var local, imported string
		typeOnly := node.IsTypeOnly
		namespace := false
		switch s := spec.(type) {
		case *parser.ImportNamedSpecifier:
			if s.Local == nil || s.Imported == nil {
				continue
			}
			local, imported = s.Local.Value, s.Imported.Value
			typeOnly = typeOnly || s.IsTypeOnly
		case *parser.ImportNamespaceSpecifier:
			if s.Local == nil {
				continue
			}
			local, namespace = s.Local.Value, true
		default:
			continue
		}
		if !namespace {
			target, ok := resolveSelfExport(exports, imported)
			if !ok {
				if named, isNamed := spec.(*parser.ImportNamedSpecifier); isNamed {
					shown := imported
					if named.Imported.Token != nil && named.Imported.Token.Type == lexer.STRING {
						shown = `"` + imported + `"`
					}
					c.addErrorWithCode(named.Imported, "TS2305", fmt.Sprintf("Module '\"%s\"' has no exported member '%s'.", node.Source.Value, shown))
				}
				c.env.Define(local, types.Any, false)
				continue
			}
			namespace = target.namespace
			if !namespace {
				if t, found := c.env.ResolveType(target.local); found && t != nil {
					c.env.DefineTypeAlias(local, t)
				}
				if !typeOnly {
					c.env.Define(local, types.Any, false)
					if c.selfImportValues == nil {
						c.selfImportValues = map[string]string{}
					}
					c.selfImportValues[local] = target.local
				}
				continue
			}
		}
		ns := types.NewNamespaceType(local)
		ns.Instantiated = true
		ns.ValueShape = types.NewObjectType()
		for name := range exports {
			target, ok := resolveSelfExport(exports, name)
			if !ok || target.namespace {
				continue
			}
			if t, found := c.env.ResolveType(target.local); found && t != nil {
				ns.TypeMembers[name] = t
			}
			ns.ValueShape.SetProperty(name, types.Any)
		}
		c.env.DefineTypeAlias(local, ns)
		if !typeOnly {
			c.env.Define(local, types.Any, false)
		}
	}
}

// reportStringModuleNames reports TS18057 for each string literal import or
// export name under --module es2015 or es2020, which cannot emit them.
func (c *Checker) reportStringModuleName(tok *lexer.Token, node parser.Node) {
	if tok == nil || tok.Type != lexer.STRING {
		return
	}
	switch c.moduleKind {
	case "es6", "es2015", "es2020":
		c.addErrorWithCode(node, "TS18057", "String literal import and export names are not supported when the '--module' flag is set to 'es2015' or 'es2020'.")
	}
}

// reportStringModuleNameExpr is reportStringModuleName for an export name.
func (c *Checker) reportStringModuleNameExpr(expr parser.Expression) {
	if lit, ok := expr.(*parser.StringLiteral); ok {
		c.reportStringModuleName(lit.Token, lit)
	}
}

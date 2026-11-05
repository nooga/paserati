package parser

import (
	"fmt"

	"github.com/nooga/paserati/pkg/lexer"
)

// Declaration spaces, as in tsc's DeclarationSpaces.
const (
	spaceType      = 1
	spaceValue     = 2
	spaceNamespace = 4
)

// exportsMergeDecl is one declaration of a name in a module or namespace body.
type exportsMergeDecl struct {
	name      *Identifier
	spaces    int
	exported  bool
	isDefault bool
}

// exportsMergeDecls lists the declarations a statement makes, with the
// declaration spaces each occupies. Ambient declarations are skipped: they are
// implicitly exported in tsc (getEffectiveDeclarationFlags).
func exportsMergeDecls(s Statement) []exportsMergeDecl {
	exported, isDefault := false, false
	if e, ok := s.(*ExportNamedDeclaration); ok {
		if e == nil || e.Declaration == nil {
			return nil
		}
		exported = true
		s = e.Declaration
	}
	one := func(name *Identifier, spaces int) []exportsMergeDecl {
		if name == nil {
			return nil
		}
		return []exportsMergeDecl{{name: name, spaces: spaces, exported: exported, isDefault: isDefault}}
	}
	if d, ok := s.(*ExportDefaultDeclaration); ok {
		if d == nil || !d.IsDeclaration {
			return nil
		}
		exported, isDefault = true, true
		switch e := d.Declaration.(type) {
		case *FunctionLiteral:
			if e != nil {
				return one(e.Name, spaceValue)
			}
		case *FunctionSignature:
			if e != nil && !e.Declare {
				return one(e.Name, spaceValue)
			}
		case *ClassExpression:
			if e != nil {
				return one(e.Name, spaceType|spaceValue)
			}
		}
		return nil
	}
	if fl, sig, ok := functionDeclInfo(s); ok {
		if fl != nil {
			return one(fl.Name, spaceValue)
		}
		if sig != nil && !sig.Declare {
			return one(sig.Name, spaceValue)
		}
		return nil
	}
	switch n := s.(type) {
	case *InterfaceDeclaration:
		if n != nil {
			return one(n.Name, spaceType)
		}
	case *TypeAliasStatement:
		if n != nil {
			return one(n.Name, spaceType)
		}
	case *ClassDeclaration:
		if n != nil && !n.Declare {
			return one(n.Name, spaceType|spaceValue)
		}
	case *NamespaceDeclaration:
		if n != nil && !n.Declare && !n.AmbientModule {
			if n.IsExported {
				exported = true
			}
			if moduleInstantiated(n.Body) {
				return one(n.Name, spaceNamespace|spaceValue)
			}
			return one(n.Name, spaceNamespace)
		}
	case *ExpressionStatement:
		if n != nil {
			if en, ok := n.Expression.(*EnumDeclaration); ok && en != nil {
				return one(en.Name, spaceType|spaceValue)
			}
		}
	case *LetStatement, *ConstStatement, *VarStatement:
		declared := false
		switch v := n.(type) {
		case *LetStatement:
			declared = v.Declare
		case *ConstStatement:
			declared = v.Declare
		case *VarStatement:
			declared = v.Declare
		}
		if declared {
			return nil
		}
		names, _ := declaredNameIdents(s)
		var out []exportsMergeDecl
		for _, id := range names {
			out = append(out, exportsMergeDecl{name: id, spaces: spaceValue, exported: exported})
		}
		return out
	}
	return nil
}

// checkExportsOnMerged is tsc's checkExportsOnMergedDeclarations for the
// declarations of one module or namespace body: declarations of a name that
// share a declaration space must all be exported or all local (TS2395), and a
// default export cannot merge with other declarations (TS2652).
func (b *binder) checkExportsOnMerged(stmts []Statement) {
	byName := map[string][]exportsMergeDecl{}
	var order []string
	conflicted := map[*lexer.Token]bool{}
	for _, d := range b.diags {
		conflicted[d.tok] = true
	}
	for _, s := range stmts {
		for _, d := range exportsMergeDecls(s) {
			// A declaration that already conflicted with another did not merge.
			if d.name.Token != nil && conflicted[d.name.Token] {
				continue
			}
			if _, seen := byName[d.name.Value]; !seen {
				order = append(order, d.name.Value)
			}
			byName[d.name.Value] = append(byName[d.name.Value], d)
		}
	}
	for _, name := range order {
		decls := byName[name]
		if len(decls) < 2 {
			continue
		}
		var exported, local, def int
		for _, d := range decls {
			switch {
			case d.isDefault:
				def |= d.spaces
			case d.exported:
				exported |= d.spaces
			default:
				local |= d.spaces
			}
		}
		commonExportLocal := exported & local
		commonDefault := def & (exported | local)
		if commonExportLocal == 0 && commonDefault == 0 {
			continue
		}
		for _, d := range decls {
			var tok *lexer.Token
			if d.name != nil {
				tok = d.name.Token
			}
			switch {
			case d.spaces&commonDefault != 0:
				b.report(tok, "TS2652", fmt.Sprintf("Merged declaration '%s' cannot include a default export declaration. Consider adding a separate 'export default %s' declaration instead.", name, name))
			case d.spaces&commonExportLocal != 0:
				b.report(tok, "TS2395", fmt.Sprintf("Individual declarations in merged declaration '%s' must be all exported or all local.", name))
			}
		}
	}
}

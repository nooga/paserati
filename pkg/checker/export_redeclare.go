package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// exportDeclKind classifies an exported declaration the way the binder's
// symbol flags do, for the merges checkExternalModuleExports looks at.
type exportDeclKind int

const (
	exportVar exportDeclKind = iota // function-scoped var
	exportBlockScoped
	exportFunction
	exportOverload
	exportClass
	exportInterface
	exportTypeAlias
	exportEnum
	exportNamespace
	exportOther // an export default expression
)

type exportedDecl struct {
	kind exportDeclKind
	name parser.Node
}

// exportKindsConflict is the binder's SymbolFlags excludes test: a
// declaration of kind k does not join a symbol already declared as other.
func exportKindsConflict(k, other exportDeclKind) bool {
	isValue := func(x exportDeclKind) bool {
		switch x {
		case exportInterface, exportTypeAlias:
			return false
		}
		return true
	}
	switch k {
	case exportVar:
		return isValue(other) && other != exportVar && other != exportNamespace
	case exportBlockScoped, exportOther:
		return isValue(other)
	case exportFunction, exportOverload:
		switch other {
		case exportFunction, exportOverload, exportClass, exportNamespace, exportInterface, exportTypeAlias:
			return false
		}
		return true
	case exportClass:
		switch other {
		case exportFunction, exportOverload, exportNamespace, exportInterface:
			return false
		}
		return true
	case exportInterface:
		return other == exportTypeAlias || other == exportEnum
	case exportTypeAlias:
		return !isValue(other) || other == exportClass || other == exportEnum
	case exportEnum:
		return other != exportEnum && other != exportNamespace
	case exportNamespace:
		return other == exportVar || other == exportBlockScoped || other == exportOther
	}
	return false
}

// checkExternalModuleExports is tsc's check that an exported name the
// binder merged from several declarations (vars, or functions with a class)
// is declared once: each declaration that is not an overload reports TS2323.
// Namespaces, enums, interfaces and a type alias beside one value may merge.
func (c *Checker) checkExternalModuleExports(program *parser.Program) {
	if program == nil || !c.IsModuleMode() {
		return
	}
	byName := map[string][]exportedDecl{}
	var order []string
	add := func(name string, d exportedDecl) {
		if name == "" || d.name == nil {
			return
		}
		decls, seen := byName[name]
		if !seen {
			order = append(order, name)
		}
		// A declaration that conflicts with the symbol gets one of its own
		// (and a duplicate-declaration error elsewhere).
		for _, prev := range decls {
			if exportKindsConflict(d.kind, prev.kind) {
				return
			}
		}
		byName[name] = append(decls, d)
	}
	var addDecl func(name string, stmt parser.Node)
	addDecl = func(name string, stmt parser.Node) {
		switch s := stmt.(type) {
		case *parser.VarStatement:
			for _, d := range s.Declarations {
				if d != nil && d.Name != nil {
					add(d.Name.Value, exportedDecl{exportVar, d.Name})
				}
			}
		case *parser.LetStatement:
			for _, d := range s.Declarations {
				if d != nil && d.Name != nil {
					add(d.Name.Value, exportedDecl{exportBlockScoped, d.Name})
				}
			}
		case *parser.ConstStatement:
			for _, d := range s.Declarations {
				if d != nil && d.Name != nil {
					add(d.Name.Value, exportedDecl{exportBlockScoped, d.Name})
				}
			}
		case *parser.DeclarationGroup:
			for _, d := range s.Declarations {
				addDecl(name, d)
			}
		case *parser.ExpressionStatement:
			addDecl(name, s.Expression)
		case *parser.FunctionLiteral:
			if s.Name != nil {
				if name == "" {
					name = s.Name.Value
				}
				add(name, exportedDecl{exportFunction, s.Name})
			}
		case *parser.FunctionSignature:
			if s.Name != nil {
				if name == "" {
					name = s.Name.Value
				}
				add(name, exportedDecl{exportOverload, s.Name})
			}
		case *parser.ClassDeclaration:
			if s.Name != nil {
				if name == "" {
					name = s.Name.Value
				}
				add(name, exportedDecl{exportClass, s.Name})
			}
		case *parser.ClassExpression:
			if s.Name != nil {
				if name == "" {
					name = s.Name.Value
				}
				add(name, exportedDecl{exportClass, s.Name})
			}
		case *parser.InterfaceDeclaration:
			if s.Name != nil && name == "" {
				add(s.Name.Value, exportedDecl{exportInterface, s.Name})
			}
		case *parser.TypeAliasStatement:
			if s.Name != nil && name == "" {
				add(s.Name.Value, exportedDecl{exportTypeAlias, s.Name})
			}
		case *parser.EnumDeclaration:
			if s.Name != nil && name == "" {
				add(s.Name.Value, exportedDecl{exportEnum, s.Name})
			}
		case *parser.NamespaceDeclaration:
			if s.Name != nil && name == "" && !s.AmbientModule {
				add(s.Name.Value, exportedDecl{exportNamespace, s.Name})
			}
		}
	}
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *parser.ExportNamedDeclaration:
			if s.Declaration != nil {
				addDecl("", s.Declaration)
			}
		case *parser.ExportDefaultDeclaration:
			if s.IsDeclaration {
				addDecl("default", s.Declaration)
			} else {
				add("default", exportedDecl{exportOther, s})
			}
		}
	}
	for _, name := range order {
		decls := byName[name]
		count, typeAlias := 0, false
		skip := false
		for _, d := range decls {
			switch d.kind {
			case exportNamespace, exportEnum:
				skip = true
			case exportTypeAlias:
				typeAlias = true
				count++
			case exportOverload, exportInterface:
			default:
				count++
			}
		}
		if skip || count <= 1 || (typeAlias && count <= 2) {
			continue
		}
		for _, d := range decls {
			if d.kind == exportOverload {
				continue
			}
			c.addErrorWithCode(d.name, errors.TS2323, fmt.Sprintf("Cannot redeclare exported variable '%s'.", name))
		}
	}
}

package parser

import (
	"testing"

	"github.com/nooga/paserati/pkg/lexer"
)

func TestJSDocAttached(t *testing.T) {
	src := `/** An interface. */
export interface Config {
    /**
     * Region to deploy in
     * @default us-east-1
     */
    region?: string;
    // not a doc
    plain: number;
}
/** Alias */
type A = {
    /** member */
    x: 1;
};
/** Klass */
export class K {
    /** field */
    @dec()
    public f = 1;
    /** method */
    m(): void {}
}
/** Func */
export function fn() {}
enum E {
    /** first */
    A,
    B,
}
`
	p := NewParser(lexer.NewLexer(src))
	prog, errs := p.ParseProgram()
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var iface *InterfaceDeclaration
	var alias *TypeAliasStatement
	var class *ClassDeclaration
	var fn *FunctionLiteral
	var enum *EnumDeclaration
	for _, st := range prog.Statements {
		d := st
		if e, ok := st.(*ExportNamedDeclaration); ok {
			d = e.Declaration
		}
		switch n := d.(type) {
		case *InterfaceDeclaration:
			iface = n
		case *TypeAliasStatement:
			alias = n
		case *ClassDeclaration:
			class = n
		case *ExpressionStatement:
			switch e := n.Expression.(type) {
			case *FunctionLiteral:
				fn = e
			case *EnumDeclaration:
				enum = e
			}
		}
	}
	check := func(what, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("%s: got %q, want %q", what, got, want)
		}
	}
	if iface == nil || alias == nil || class == nil || fn == nil || enum == nil {
		t.Fatalf("missing declaration: %v %v %v %v %v", iface, alias, class, fn, enum)
	}
	check("interface", iface.Doc, "An interface.")
	check("region", iface.Properties[0].Doc, "Region to deploy in\n@default us-east-1")
	check("plain", iface.Properties[1].Doc, "")
	check("alias", alias.Doc, "Alias")
	check("class", class.Doc, "Klass")
	check("field", class.Body.Properties[0].Doc, "field")
	check("method", class.Body.Methods[0].Doc, "method")
	check("func", fn.Doc, "Func")
	check("enum member", enum.Members[0].Doc, "first")
	check("enum member 2", enum.Members[1].Doc, "")
	if ot, ok := alias.Type.(*ObjectTypeExpression); ok {
		check("type literal member", ot.Properties[0].Doc, "member")
	} else {
		t.Errorf("alias type is %T", alias.Type)
	}
}

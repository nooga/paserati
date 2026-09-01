package parser

import (
	"fmt"
	"testing"

	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/source"
)

func parseStatementsForTest(t *testing.T, input string) []Statement {
	t.Helper()
	sourceFile := source.NewEvalSource(input)
	p := NewParser(lexer.NewLexerWithSource(sourceFile))
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) != 0 {
		for _, err := range parseErrs {
			t.Errorf("  %s", err.Error())
		}
		t.Fatalf("parsing %q produced %d errors", input, len(parseErrs))
	}
	return program.Statements
}

// stmtShape names a statement by kind and, for declarations, the bindings it
// introduces - enough to pin the desugaring without asserting on register
// allocation or bytecode.
func stmtShape(stmt Statement) string {
	switch s := stmt.(type) {
	case *LetStatement:
		return "let " + declaratorNames(s.Declarations)
	case *ConstStatement:
		return "const " + declaratorNames(s.Declarations)
	case *VarStatement:
		return "var " + declaratorNames(s.Declarations)
	case *ObjectDestructuringDeclaration:
		return s.Token.Literal + " {…}"
	case *ArrayDestructuringDeclaration:
		return s.Token.Literal + " […]"
	case *ExportNamedDeclaration:
		return "export " + stmtShape(s.Declaration)
	case *DeclarationGroup:
		out := "group("
		for i, d := range s.Declarations {
			if i > 0 {
				out += ", "
			}
			out += stmtShape(d)
		}
		return out + ")"
	default:
		return fmt.Sprintf("%T", stmt)
	}
}

func declaratorNames(decls []*VarDeclarator) string {
	out := ""
	for i, d := range decls {
		if i > 0 {
			out += ","
		}
		if d == nil || d.Name == nil {
			out += "<nil>"
			continue
		}
		out += d.Name.Value
	}
	return out
}

// TestDeclaratorListShapes pins how a let/const/var declarator list desugars.
// A pattern is legal in any declarator position (paserati#159, #160); the
// pre-existing single-statement shapes must survive unchanged, and only a
// genuinely mixed clause becomes a DeclarationGroup - spliced into the
// enclosing statement list, so a mixed clause in statement position yields
// several sibling statements rather than a group.
func TestDeclaratorListShapes(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		// Unchanged shapes: no pattern, or a pattern on its own.
		{"let a = 1;", []string{"let a"}},
		{"let a = 1, b = 2, c = 3;", []string{"let a,b,c"}},
		{"const a = 1, b = 2;", []string{"const a,b"}},
		{"var a = 1, b;", []string{"var a,b"}},
		{"let {a} = o;", []string{"let {…}"}},
		{"let [a] = o;", []string{"let […]"}},
		{"const {a} = o;", []string{"const {…}"}},

		// #159: pattern after a comma. Spliced into the statement list.
		{"let a = 1, {b} = o;", []string{"let a", "let {…}"}},
		{"let a = 1, [b] = o;", []string{"let a", "let […]"}},
		{"const a = 1, {b} = o;", []string{"const a", "const {…}"}},
		{"var a = 1, {b} = o;", []string{"var a", "var {…}"}},
		{"let a, {b} = o;", []string{"let a", "let {…}"}},

		// #160: pattern first, plain bindings after.
		{"let {a} = o, b = 2;", []string{"let {…}", "let b"}},
		{"let {a} = o, b;", []string{"let {…}", "let b"}},
		{"const {a} = o, b = 2;", []string{"const {…}", "const b"}},

		// Several patterns, and a pattern in the middle. One statement per
		// declarator, in source order.
		{"let {a} = o, {b} = o;", []string{"let {…}", "let {…}"}},
		{"let a = 1, {b} = o, c = 3;", []string{"let a", "let {…}", "let c"}},

		// Statements after the clause are unaffected by the splice.
		{"let a = 1, {b} = o; let c = 2;", []string{"let a", "let {…}", "let c"}},

		// Each desugared declaration is exported in its own right.
		{"export let a = 1, {b} = o;", []string{"export let a", "export let {…}"}},
		{"export let {a} = o, b = 2;", []string{"export let {…}", "export let b"}},
	}

	for _, tt := range tests {
		stmts := parseStatementsForTest(t, tt.input)
		got := make([]string, 0, len(stmts))
		for _, s := range stmts {
			got = append(got, stmtShape(s))
		}
		if fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("%q\n got: %v\nwant: %v", tt.input, got, tt.want)
		}
	}
}

// TestForInitDeclaratorListShapes pins the same desugaring in a `for`
// initializer. A for initializer must be a single Statement, so this is the one
// position where a DeclarationGroup survives rather than being spliced.
func TestForInitDeclaratorListShapes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"for (let i = 0; ; ) {}", "let i"},
		{"for (let i = 0, j = 1; ; ) {}", "let i,j"},
		{"for (var i = 0, j = 1; ; ) {}", "var i,j"},
		{"for (const i = 0, j = 1; ; ) {}", "const i,j"},
		{"for (let {a} = o; ; ) {}", "let {…}"},
		{"for (let i = 0, {a} = o; ; ) {}", "group(let i, let {…})"},
		{"for (let {a} = o, i = 0; ; ) {}", "group(let {…}, let i)"},
		{"for (var i = 0, [a] = o; ; ) {}", "group(var i, var […])"},
		{"for (const {a} = o, i = 0; ; ) {}", "group(const {…}, const i)"},
	}

	for _, tt := range tests {
		stmts := parseStatementsForTest(t, tt.input)
		if len(stmts) != 1 {
			t.Errorf("%q: expected 1 statement, got %d", tt.input, len(stmts))
			continue
		}
		forStmt, ok := stmts[0].(*ForStatement)
		if !ok {
			t.Errorf("%q: expected *ForStatement, got %T", tt.input, stmts[0])
			continue
		}
		if got := stmtShape(forStmt.Initializer); got != tt.want {
			t.Errorf("%q initializer\n got: %s\nwant: %s", tt.input, got, tt.want)
		}
	}
}

// TestForOfInDeclaratorHeadUnaffected guards the lookahead that distinguishes a
// for-of/for-in head (exactly one binding, no initializer) from a regular for
// initializer, which the shared declarator-list parser must not disturb.
func TestForOfInDeclaratorHeadUnaffected(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"for (let x of xs) {}", "let x"},
		{"for (const {a} of xs) {}", "const {…}"},
		{"for (const [a, b] of xs) {}", "const […]"},
		{"for (var k in obj) {}", "var k"},
		{"for (let {a} in obj) {}", "let {…}"},
	}

	for _, tt := range tests {
		stmts := parseStatementsForTest(t, tt.input)
		if len(stmts) != 1 {
			t.Errorf("%q: expected 1 statement, got %d", tt.input, len(stmts))
			continue
		}
		var variable Statement
		switch s := stmts[0].(type) {
		case *ForOfStatement:
			variable = s.Variable
		case *ForInStatement:
			variable = s.Variable
		default:
			t.Errorf("%q: expected a for-of/for-in statement, got %T", tt.input, stmts[0])
			continue
		}
		if got := stmtShape(variable); got != tt.want {
			t.Errorf("%q head\n got: %s\nwant: %s", tt.input, got, tt.want)
		}
	}
}

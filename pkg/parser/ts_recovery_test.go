package parser

import (
	"reflect"
	"testing"

	"github.com/nooga/paserati/pkg/lexer"
)

// parseCodes parses src and returns "line:code" for every reported diagnostic.
func parseCodes(src string) []string {
	p := NewParser(lexer.NewLexer(src))
	_, errs := p.ParseProgram()
	var out []string
	for _, e := range errs {
		out = append(out, itoa(e.Pos().Line)+":"+e.Code())
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestTSStyleRecovery pins the diagnostics (line:code) tsc gives for small
// broken programs: one error per mistake, with tsc's recovery.
func TestTSStyleRecovery(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"missing operand before binary operator", "var v = || b;", []string{"1:TS1109"}},
		{"empty parenthesized call", "var v = ()({});", []string{"1:TS1109"}},
		{"missing comma in argument list", "f(1 2);", []string{"1:TS1005"}},
		{"stray close paren at statement start", "var a = 1;\n)\nvar b = 2;", []string{"2:TS1128"}},
		{"missing comma in object literal", "var v = { a: 1 b: 2 }", []string{"1:TS1005"}},
		{"empty declaration list", "var;\n", []string{"1:TS1123"}},
		{"destructuring declaration needs initializer", "var [a, b];", []string{"1:TS1182"}},
		{"return outside function", "return 1;", []string{"1:TS1108"}},
		{"return plus syntax error suppresses grammar error", "return 1;\nvar x = ;", []string{"2:TS1109"}},
		{"break outside loop", "break;", []string{"1:TS1105"}},
		{"continue outside loop", "continue;", []string{"1:TS1104"}},
		{"break to missing label", "while (true) { break nope; }", []string{"1:TS1116"}},
		{"break across function boundary", "while (true) { (() => { break; })(); }", []string{"1:TS1107"}},
		{"unary before exponent", "var x = -2 ** 2;", []string{"1:TS17006"}},
		{"bad unicode escape in string", "var x = \"\\u{110000}\";", []string{"1:TS1198"}},
		{"hex digit expected in string", "var x = \"\\xg\";", []string{"1:TS1125"}},
		{"unterminated string", "var x = \"abc\nvar y = 1;", []string{"1:TS1002"}},
		{"unterminated template", "var x = `abc", []string{"1:TS1160"}},
		{"invalid escape in untagged template", "var x = `\\u{hello}`;", []string{"1:TS1125"}},
		{"tagged template tolerates invalid escapes", "tag`\\u{hello}`;", nil},
		{"exponent needs a digit", "var x = 1e;", []string{"1:TS1124"}},
		{"const needs initializer", "const c;", []string{"1:TS1155"}},
		{"ambient const needs no initializer", "declare namespace N { const c: number; }", nil},
		{"getter with parameters", "class C { get x(a) { return 1; } }", []string{"1:TS1054"}},
		{"setter with no parameter", "class C { set x() { } }", []string{"1:TS1049"}},
		{"misplaced async modifier", "async class C {}", []string{"1:TS1042"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCodes(tc.src)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("source %q:\n got  %v\n want %v", tc.src, got, tc.want)
			}
		})
	}
}

// TestTSSyntaxAcceptedMore covers constructs tsc accepts that the parser used to
// reject.
func TestTSSyntaxAcceptedMore(t *testing.T) {
	inputs := []string{
		"declare module \"foo\";",
		"const x = async => async;",
		"declare function as(...args: any[]);",
		"class C { get<K>(k: K) { return k; } set<K>(k: K) { } }",
		"declare class D { get a(): string; set a(v: string); }",
		"class E { constructor(y: any)\n constructor(x: number) {} }",
		"type T = { readonly [n: number]: string };",
		"declare var [a, b];",
		"declare const enum E1 { a, b }",
		"class F implements A.B, C<D> {}",
		"type V<in out T> = { f: (x: T) => T };",
		"let q = <const> 10;",
		"declare function assert(v: unknown): asserts v;\ndeclare function isS(v: unknown): asserts v is string;",
		"class G { constructor(public override p: string) {} }",
		"function f({ \"show\": r = 5 }: { show?: number }) { return r; }",
		"const h = function (this, ...args) { return args.length; };",
		"const g = async <U, R>(x: U, y: R) => x;",
		"type M<T> = { [P in keyof T as P]: T[P] };",
		"type Q = typeof Array<number>;",
		"for (foo().x of []) {}",
		"function f(a: number, this: Window) {}",
		"@dec(1) class H { m(@a x: number, @b() y: string) {} }",
		"class I { @x! m() {} @g<number>() n() {} }",
		"interface J { foo(cb: (a: any) => void): J\n is(): boolean; }",
		"type X<T> = T extends [infer H extends string, ...infer R] ? H : never;",
		"type Y<T> = T extends infer U extends string ? U : never;",
	}
	for _, in := range inputs {
		if got := parseCodes(in); len(got) != 0 {
			// A handful of these are reported as semantic/grammar errors by tsc (the
			// parameter-property and this-position diagnostics); anything else is a
			// parse failure.
			for _, c := range got {
				if c != "1:TS2680" && c != "1:TS2369" {
					t.Errorf("input %q: unexpected diagnostics %v", in, got)
					break
				}
			}
		}
	}
}

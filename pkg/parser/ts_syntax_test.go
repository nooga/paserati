package parser

import (
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/lexer"
)

// TestTypeScriptDeclarationSyntaxAccepted checks constructs found in
// TypeScript's own lib.d.ts files parse without any syntax error.
func TestTypeScriptDeclarationSyntaxAccepted(t *testing.T) {
	inputs := []string{
		// this parameters in signatures and function types
		"declare function f(this: Window, a: number): void;",
		"type F = (this: T, ...args: A) => R;",
		"interface I { flat<A>(\n this: A,\n depth?: number,\n): A[]; }",
		// contextual keywords as parameter / property names
		"type F = (type: string, from: number, set?: boolean, get?, of?: any, declare?: 1, module?: 2, namespace?: 3) => void;",
		"interface Q { type: string; from: number; get: 1; set: 2; of: 3; declare: 4; module: 5; namespace: 6; delete(): void; }",
		"interface I { m(type: string, from: number, readonly: boolean): void }",
		// rest with a binding pattern
		"interface G { next(...[value]: [] | [number]): void; }",
		// this type predicates
		"interface I { isFoo(): this is Foo; m(x: unknown): x is string; }",
		// ambient const without initializer
		"declare namespace Intl { const PluralRules: PluralRulesConstructor; }",
		// qualified generic heritage and type references
		"interface I<T, R, N> extends globalThis.IteratorObject<T, R, N> {}",
		"declare namespace N { class C<T> {} }\ntype A = N.C<string>;",
		// unique symbol
		"declare const sym: unique symbol;",
		// infer with constraint
		"type X<T> = T extends [infer H extends string, ...infer R] ? H : never;",
		"type Y<T> = T extends infer U extends string ? U : never;",
		// nested conditional types in the true branch
		"type Z<T> = T extends A ? T extends B ? 1 : 2 : 3;",
		// variadic tuple types
		"type V<A extends any[], B extends any[]> = [...A, ...B];",
		"declare function bind<T, A extends any[], B extends any[], R>(this: (this: T, ...args: [...A, ...B]) => R, thisArg: T, ...args: A): (...args: B) => R;",
		// parenthesized type that is a bare name
		"type P = (Foo)[];",
	}
	for _, in := range inputs {
		p := NewParser(lexer.NewLexer(in))
		_, errs := p.ParseProgram()
		if len(errs) > 0 {
			t.Errorf("input %q: unexpected errors: %v", in, errs)
		}
	}
}

func TestParenthesizedBareTypeNameIsNotAnyParam(t *testing.T) {
	p := NewParser(lexer.NewLexer("type P = (Foo)[];"))
	prog, errs := p.ParseProgram()
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	got := prog.Statements[0].String()
	if !strings.Contains(got, "Foo") || strings.Contains(got, "any") {
		t.Errorf("(Foo)[] should keep the name Foo, got %q", got)
	}
}

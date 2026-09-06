package driver

import (
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// issue278Extname mirrors a real Node builtin like path.extname: a
// fixed-arity, non-variadic Go function that only cares about its first
// argument. Real JS callers routinely pass such a function around as a bare
// value and invoke it from generic machinery that always supplies a fixed
// argument count regardless of what the callee declares - the textbook case
// being Array.prototype.forEach, which always calls back with exactly 3
// arguments (element, index, array) per spec.
func issue278Extname(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
}

type issue278Thing struct{ Value string }

// newIssue278Thing only declares one parameter; called as a constructor
// with more arguments than that (paserati#278's createClassConstructor
// path) must silently ignore the extras, exactly like a plain Function.
func newIssue278Thing(input string) (*issue278Thing, error) {
	return &issue278Thing{Value: input}, nil
}

// Greet only declares one parameter; called as a bound method with more
// arguments than that (paserati#278's createBoundMethod path) must silently
// ignore the extras too.
func (t *issue278Thing) Greet(suffix string) string {
	return t.Value + suffix
}

// TestModuleFunctionExtraArgsIgnored covers paserati#278: goFunctionToVM's
// non-variadic branch sized its reflect.Value argument slice to len(args) -
// the number of arguments the JS caller passed - rather than to the Go
// function's own declared arity, then only filled indices below that arity.
// When a JS call site passed MORE arguments than the Go function declares,
// the tail of that slice stayed as unset reflect.Value{} zero values and
// reflect.Value.Call panicked outright ("too many input arguments") instead
// of silently dropping the extras like real JS/Go interop should.
func TestModuleFunctionExtraArgsIgnored(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("issue278mod", func(m *ModuleBuilder) {
		m.Function("extname", issue278Extname)
		m.Class("Thing", &issue278Thing{}, newIssue278Thing)
	})

	res, errs := p.RunString(`
		import { extname, Thing } from "issue278mod";
		const results = ["a.ts", "b.txt", "noext"].map(extname);

		const t = new Thing("hi", "ignored1", "ignored2");
		const greeted = t.greet("!", "ignored1", "ignored2");

		JSON.stringify({ results, thingValue: t.Value, greeted });
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got := res.ToString()
	want := `{"results":[".ts",".txt",""],"thingValue":"hi","greeted":"hi!"}`
	if got != want {
		t.Fatalf("extra JS args to a fixed-arity Go function should be silently ignored: got %s, want %s", got, want)
	}
}

// TestModuleFunctionForEachCallback is the exact repro from the issue: a
// fixed-arity native module function passed directly as an
// Array.prototype.forEach callback, which per spec always invokes it with
// 3 arguments regardless of what the callback declares.
func TestModuleFunctionForEachCallback(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("issue278mod2", func(m *ModuleBuilder) {
		m.Function("extname", issue278Extname)
	})

	res, errs := p.RunString(`
		import { extname } from "issue278mod2";
		// The exact real-world shape from the issue: a fixed-arity native
		// function passed directly as a bare callback, no wrapper -
		// Array.prototype.map (like forEach) always calls back with 3
		// arguments (element, index, array) regardless of what the callback
		// declares.
		JSON.stringify(["a.ts", "b.txt"].map(extname));
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got := res.ToString()
	want := `[".ts",".txt"]`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// TestWrapGoFunctionExtraArgsIgnored covers the same overflow bug in
// ValueConverter.wrapGoFunction (used when a plain Go function value is
// converted via the reflection-based ValueConverter path rather than
// ModuleBuilder.Function).
func TestWrapGoFunctionExtraArgsIgnored(t *testing.T) {
	vc := &ValueConverter{}
	fnVal := vc.wrapGoFunction(issue278Extname)
	if fnVal.Type() == vm.TypeUndefined {
		t.Fatal("wrapGoFunction returned undefined for a valid function")
	}

	nf := fnVal.AsNativeFunction()
	result, err := nf.Fn([]vm.Value{
		vm.NewString("a.ts"),
		vm.NewString("extra1"),
		vm.NewString("extra2"),
	})
	if err != nil {
		t.Fatalf("unexpected error calling with extra args: %v", err)
	}
	if got := result.ToString(); got != ".ts" {
		t.Fatalf("got %q, want %q", got, ".ts")
	}
}

package driver

import "testing"

type issue512Counter struct{ n int }

func (c *issue512Counter) Increment() int { c.n++; return c.n }

func newIssue512Counter() *issue512Counter { return &issue512Counter{} }

// TestClassMethodsLiveOnSharedPrototype is a regression test for
// paserati#512: ModuleBuilder.Class bound Go struct methods directly onto
// each *instance* (bindStructMethods, called from createClassConstructor)
// rather than onto the class's shared .prototype object, which itself
// carried only `constructor`. This was invisible for `new X()` followed by
// calling a method on that same instance, since the method was an own
// property either way, but it broke ordinary JS prototype semantics beyond
// that: a method read directly off the prototype, or another constructor
// borrowing the class's prototype for its own instances
// (`Other.prototype = X.prototype`) - a normal, legal idiom real published
// code (iconv-lite's string_decoder shim) actually relies on.
func TestClassMethodsLiveOnSharedPrototype(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	p.DeclareModule("issue512mod", func(m *ModuleBuilder) {
		m.Class("Counter", &issue512Counter{}, newIssue512Counter)
	})

	res, errs := p.RunString(`
		import { Counter } from "issue512mod";

		function Other() {}
		Other.prototype = Counter.prototype;
		const o = new Other();

		JSON.stringify({
			methodOnPrototype: typeof (Counter as any).prototype.increment === "function",
			borrowedTypeof: typeof (o as any).increment,
			instanceStillWorks: (() => {
				const c = new Counter();
				(c as any).increment();
				(c as any).increment();
				return (c as any).increment();
			})(),
		});
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got := res.ToString()
	want := `{"methodOnPrototype":true,"borrowedTypeof":"function","instanceStillWorks":3}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// TestClassMethodCallThroughBorrowedPrototype covers the exact failure mode
// from iconv-lite: an instance built through the class's own constructor
// (so it carries the internal Go state createPrototypeMethod needs) must
// still be callable via a method reached only through prototype borrowing,
// not just via `new X()`'s own-property shortcut.
func TestClassMethodCallThroughBorrowedPrototype(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	p.DeclareModule("issue512mod2", func(m *ModuleBuilder) {
		m.Class("Counter", &issue512Counter{}, newIssue512Counter)
	})

	res, errs := p.RunString(`
		import { Counter } from "issue512mod2";

		function Other() {}
		Other.prototype = Counter.prototype;

		// A real Counter instance, but invoked through Other's borrowed
		// prototype reference rather than Counter's own.
		const c = new Counter();
		const incrementViaOther = Other.prototype.increment;
		incrementViaOther.call(c);
		incrementViaOther.call(c);
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if res.ToString() != "2" {
		t.Fatalf("expected '2', got %s", res.ToString())
	}
}

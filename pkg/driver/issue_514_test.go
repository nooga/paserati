package driver

import "testing"

type issue514Counter struct{ n int }

func (c *issue514Counter) Increment() int { c.n++; return c.n }

func newIssue514Counter(start float64) *issue514Counter {
	return &issue514Counter{n: int(start)}
}

// TestClassConstructorCallInitializesThisInPlace is a regression test for
// paserati#514: a ModuleBuilder.Class constructor invoked as a plain function
// with an explicit receiver - the classic ES5 parent-constructor call
// `Parent.call(this, ...)`, used by iconv-lite's InternalDecoder against a
// host-provided StringDecoder - always built and returned a fresh object,
// leaving the caller's `this` without the Go state its prototype methods
// need.
func TestClassConstructorCallInitializesThisInPlace(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	p.DeclareModule("issue514mod", func(m *ModuleBuilder) {
		m.Class("Counter", &issue514Counter{}, newIssue514Counter)
	})

	res, errs := p.RunString(`
		import { Counter } from "issue514mod";

		function Other(start) {
			this.ret = Counter.call(this, start);
		}
		Other.prototype = Counter.prototype;
		const o = new Other(5);
		o.increment();

		function Applied(start) { Counter.apply(this, [start]); }
		Applied.prototype = Counter.prototype;
		const a = new Applied(10);

		const fresh = new Counter(1);
		const bare = Counter(2);

		let arrayErr = "";
		try { Counter.prototype.increment.call([]); } catch (e) { arrayErr = e.message; }

		JSON.stringify({
			inPlace: o.increment(),
			callReturns: typeof o.ret,
			instanceofCounter: o instanceof Counter,
			applied: a.increment(),
			fresh: fresh.increment(),
			bare: bare.increment(),
			arrayErr,
		});
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got := res.ToString()
	want := `{"inPlace":7,"callReturns":"undefined","instanceofCounter":true,"applied":11,"fresh":2,"bare":3,"arrayErr":"method called on an object that is not a valid instance of this class"}`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

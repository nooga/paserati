package driver

import "testing"

// TestAsyncFunctionRestParamAwaited reproduces the bug this file's sibling
// smoke tests (tests/scripts/async_rest_param_identity.ts,
// async_rest_param_mixed.ts) can only cover for the synchronously-resolved
// case: executeAsyncFunctionBody (pkg/vm/async.go) built its initial frame
// by hand instead of going through prepareCall (call.go), and never
// reproduced prepareCall's variadic/rest-parameter handling. A rest-only
// async function called with fewer arguments than its arity left the rest
// register at Undefined instead of an empty array - `await af()` returned
// `undefined` where `[]` was expected. This exercises the exact repro from
// the bug report, through an actual `await`, using RunCode's support for a
// resolved top-level-await result rather than a pending Promise.
func TestAsyncFunctionRestParamAwaited(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		async function af(...args) { return args; }
		const a1 = await af();
		Array.isArray(a1) && a1.length === 0
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !result.IsTruthy() {
		t.Errorf("expected await af() to resolve to an empty array, got %v", result.ToString())
	}
}

// TestAsyncFunctionRestParamFreshIdentity covers the same A4-shaped identity
// requirement as tests/scripts/rest_param_fresh_identity.ts, but for the
// async call path (executeAsyncFunctionBody), which builds its rest array
// separately from the ordinary/sync call path this fix mirrors.
func TestAsyncFunctionRestParamFreshIdentity(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		async function af(...args) { return args; }
		const a1 = await af();
		const a2 = await af();
		a1.push(42);
		a1 !== a2 && a2.length === 0
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !result.IsTruthy() {
		t.Errorf("expected two await af() calls to return distinct, independent arrays, got %v", result.ToString())
	}
}

// TestAsyncFunctionRestParamMixedAwaited covers a rest parameter alongside
// preceding fixed parameters, with extra arguments actually present, rather
// than only the fully-empty case.
func TestAsyncFunctionRestParamMixedAwaited(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		async function afMixed(a, ...rest) { return [a, rest]; }
		const r = await afMixed(1, 2, 3);
		r[0] === 1 && Array.isArray(r[1]) && r[1].length === 2 && r[1][0] === 2 && r[1][1] === 3
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !result.IsTruthy() {
		t.Errorf("expected afMixed(1, 2, 3) to resolve to [1, [2, 3]], got %v", result.ToString())
	}
}

package driver

import "testing"

// TestAsyncFunctionArgumentsLength reproduces the reported bug directly:
// executeAsyncFunctionBody (pkg/vm/async.go) builds an async function's
// initial frame by hand instead of going through prepareCall (call.go), and
// never set frame.args - the field OpGetArguments actually reads to build
// the `arguments` object; frame.argCount only sizes the separate
// register-copy loop. `arguments.length` read 0 regardless of the actual
// call. Uses RunCode's support for a resolved top-level-await result rather
// than a pending Promise, matching the sibling rest-parameter tests in this
// package.
func TestAsyncFunctionArgumentsLength(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		async function first(a, b, c) { return arguments.length; }
		const n = await first(1, 2, 3);
		n === 3
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !result.IsTruthy() {
		t.Errorf("expected await first(1, 2, 3) to resolve arguments.length to 3, got %v", result.ToString())
	}
}

// TestAsyncFunctionArgumentsIndexing covers indexed reads/writes and mapped
// (sloppy-mode, simple-parameter-list) aliasing between `arguments[i]` and
// its corresponding parameter binding - the frame.args fix must supply
// OpGetArguments with the real argument values, not just the right count.
// Mapped indices within the declared arity alias live registers (already
// correctly populated by the preceding rest-parameter fix) rather than
// reading frame.args, so `extraIndexed` - an argument beyond the function's
// arity, necessarily unmapped - is the part of this test that actually
// discriminates the frame.args fix; the rest holds either way but is worth
// asserting alongside it.
func TestAsyncFunctionArgumentsIndexing(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		async function f(a, b) {
			const before = arguments[0] === 1 && arguments[1] === 2;
			arguments[0] = 99;
			const aliasedByArgWrite = a === 99;
			a = 7;
			const aliasedByParamWrite = arguments[0] === 7;
			return before && aliasedByArgWrite && aliasedByParamWrite;
		}
		async function extra(a) { return arguments[1]; }
		const extraIndexed = (await extra(1, 2)) === 2;
		(await f(1, 2)) && extraIndexed
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !result.IsTruthy() {
		t.Errorf("expected mapped and unmapped arguments indexing to hold in an async function, got %v", result.ToString())
	}
}

// TestAsyncFunctionArgumentsCallee covers arguments.callee, since this fix
// also set frame.calleeValue (mirroring prepareCall) - a field
// OpGetArguments only falls back to frame.closure for when it's
// TypeUndefined, so setting it to the wrong value would be worse than
// leaving it unset. Covers both a direct call and a bound call, where
// arguments.callee must still name the original target function, not the
// bound wrapper.
func TestAsyncFunctionArgumentsCallee(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		async function f(a) { return arguments.callee === f; }
		const direct = await f(1);

		async function g() { return arguments.callee.name; }
		const bound = g.bind(null);
		const boundNamesTarget = (await bound()) === "g";

		direct && boundNamesTarget
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !result.IsTruthy() {
		t.Errorf("expected arguments.callee to identify the original function for both direct and bound async calls, got %v", result.ToString())
	}
}

// TestAsyncFunctionArgumentsNotStaleAcrossFrameReuse covers the slot-reuse
// hazard this bug's fix is adjacent to: vm.frames is a fixed array reused by
// index, so a first call's argument state must not leak into a later,
// differently-shaped call landing on the same physical slot.
func TestAsyncFunctionArgumentsNotStaleAcrossFrameReuse(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		function sync5(a, b, c, d, e) { return arguments.length; }
		async function asyncFew(x) { return arguments.length; }
		sync5(1, 2, 3, 4, 5);
		await asyncFew(1)
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if result.ToString() != "1" {
		t.Errorf("expected asyncFew(1)'s arguments.length to read 1, not a stale count from the prior sync5 call, got %v", result.ToString())
	}
}

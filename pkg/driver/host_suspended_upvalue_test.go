package driver

import (
	"strconv"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// These tests cover #247: a local captured by a nested closure inside an
// async function or generator must keep its correct value across a
// suspend/resume (await/yield), even while other code runs concurrently and
// reuses the register-stack slice the suspended frame's own registers were
// carved from.
//
// Root cause: a generator/async frame's `openUpvalues` are deliberately kept
// open (not closed) across a yield/await, because a suspended closure over a
// `let`/`const` must keep observing the binding once the function resumes.
// But `frame.registers` is a slice of the single shared vm.registerStack,
// and suspending immediately hands that exact stack region back for reuse by
// whatever unrelated code runs next - so an open upvalue whose Location
// still points into it silently starts aliasing that unrelated code's data
// the moment anything else reuses the slot. See vm.relocateOpenUpvalues's own
// doc comment for the fix.

// TestAsyncClosureSurvivesAwaitUnderConcurrentStackReuse is a from-scratch
// isolation of the issue with plain async functions only (no generators, no
// $262) - simpler to reproduce than the issue's own repro and a better
// regression test for it.
func TestAsyncClosureSurvivesAwaitUnderConcurrentStackReuse(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		function pollute(n) {
			// Recurse to build up a register-heavy call chain that occupies
			// whatever register-stack region gets freed up while producer() is
			// suspended at its await.
			if (n <= 0) return 0;
			let a = "junk-A-" + n;
			let b = "junk-B-" + n;
			let c = { tag: "junk-C-" + n };
			let d = [n, n, n];
			let sym = Symbol("junk-sym-" + n);
			return pollute(n - 1) + (a.length - a.length) + (b.length - b.length) +
				(c ? 0 : 0) + d.length - d.length + (sym ? 0 : 0) + 1;
		}

		async function producer() {
			let obj = { tag: "original-obj" };
			const getObj = () => obj; // arrow captures 'obj' via a register upvalue
			await new Promise((r) => setTimeout(r, 0)); // suspend here
			return getObj();
		}

		let result = null;
		async function main() {
			const p = producer();
			for (let i = 0; i < 20; i++) {
				pollute(30);
			}
			result = await p;
		}
		main();
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}

	result, ok := p.GetVM().GetGlobal("result")
	if !ok {
		t.Fatal("global result not found")
	}
	if !result.IsObject() {
		t.Fatalf("expected the original object back, got %s (%s) - the closure's captured local was corrupted across the await", result.Inspect(), result.TypeName())
	}
	tag, exists := result.AsPlainObject().GetOwn("tag")
	if !exists || tag.ToString() != "original-obj" {
		t.Errorf("expected tag %q, got %s", "original-obj", tag.Inspect())
	}
}

// TestAsyncClosureMutationAfterResumeStaysCoherent guards against a *wrong*
// fix for the above: naively closing a suspending frame's open upvalues
// (rather than re-homing them) would make a captured local mutated again
// after resume desync between a direct register write and a closure's view
// of the same binding, since paserati's compiler keeps writing captured
// locals directly to the register for the rest of the enclosing function
// (only outer closures go through the upvalue) - see
// freeScopeRegisters/captureUpvalue's doc comments.
func TestAsyncClosureMutationAfterResumeStaysCoherent(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		async function f() {
			let n = 1;
			const get = () => n;
			const inc = () => { n++; };
			await new Promise((r) => setTimeout(r, 0));
			inc();
			n = n + 10;
			return [n, get()];
		}
		let result = null;
		f().then((r) => { result = r; });
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}

	result, ok := p.GetVM().GetGlobal("result")
	if !ok || !result.IsArray() {
		t.Fatalf("expected array result, got %v", result)
	}
	arr := result.AsArray()
	if arr.Length() != 2 || arr.Get(0).ToString() != "12" || arr.Get(1).ToString() != "12" {
		t.Errorf("expected [12, 12] (direct write and closure read must agree), got %s", result.Inspect())
	}
}

// TestGeneratorClosureMutationAcrossYieldsStaysCoherent is the generator
// equivalent of the above: a captured local mutated between two yields, read
// via the same closure after each.
func TestGeneratorClosureMutationAcrossYieldsStaysCoherent(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		function* g() {
			let n = 1;
			const get = () => n;
			yield get();
			n = n + 10;
			yield get();
			n = n + 100;
			yield get();
		}
		let it = g();
		[it.next().value, it.next().value, it.next().value]
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !result.IsArray() {
		t.Fatalf("expected array result, got %s", result.Inspect())
	}
	arr := result.AsArray()
	got := [3]string{arr.Get(0).ToString(), arr.Get(1).ToString(), arr.Get(2).ToString()}
	want := [3]string{"1", "11", "111"}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestIssue247Repro is the issue's own minimal repro: a `const` captured by
// a closure in a concurrently-running async IIFE must not lose its value
// across an await while a separately-driven async generator (with its own
// internal await, consumed via `for await`) is live at the same time.
func TestIssue247Repro(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		class EventStream {
			queue = [];
			waiting = [];
			done = false;
			push(event) {
				const waiter = this.waiting.shift();
				if (waiter) { waiter({ value: event, done: false }); }
				else { this.queue.push(event); }
			}
			end() {
				this.done = true;
				while (this.waiting.length > 0) {
					this.waiting.shift()({ value: undefined, done: true });
				}
			}
			async *[Symbol.asyncIterator]() {
				while (true) {
					if (this.queue.length > 0) { yield this.queue.shift(); }
					else if (this.done) { return; }
					else {
						const result = await new Promise((resolve) => this.waiting.push(resolve));
						if (result.done) return;
						yield result.value;
					}
				}
			}
		}

		var typeofBlocks = null;
		var isArrayBlocks = null;
		function startStreaming() {
			const stream = new EventStream();
			(async () => {
				const output = { content: [] };
				const blocks = output.content;
				const getContentIndex = (block) => {
					typeofBlocks = typeof blocks;
					isArrayBlocks = Array.isArray(blocks);
					return -1;
				};
				await new Promise((r) => setTimeout(r, 0));
				const block = {};
				getContentIndex(block);
				stream.push({ type: "done" });
				stream.end();
			})();
			return stream;
		}

		let doneFlag = false;
		async function main() {
			const stream = startStreaming();
			for await (const event of stream) {}
			doneFlag = true;
		}
		main();
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}

	done, ok := p.GetVM().GetGlobal("doneFlag")
	if !ok || !done.IsTruthy() {
		t.Fatalf("expected doneFlag to be true (for-await loop must complete), got %v", done)
	}
	typeofBlocks, ok := p.GetVM().GetGlobal("typeofBlocks")
	if !ok || typeofBlocks.Type() == vm.TypeNull {
		t.Fatal("typeofBlocks never set - getContentIndex never ran")
	}
	if typeofBlocks.ToString() != "object" {
		t.Errorf("expected typeof blocks === \"object\" (real Node behavior), got %q", typeofBlocks.ToString())
	}
	isArrayBlocks, ok := p.GetVM().GetGlobal("isArrayBlocks")
	if !ok || !isArrayBlocks.IsTruthy() {
		t.Errorf("expected Array.isArray(blocks) === true, got %v", isArrayBlocks)
	}
}

// TestIssue247ReproLarger is the larger, closer-to-production repro from a
// follow-up comment on #247 (~80 simulated SSE chunks via an async generator,
// a producer IIFE mutating a captured `thinkingBlock`/`blocks` across many
// awaits). Where the minimal repro above lost the binding to `undefined`,
// this shape corrupted it to a live but *unrelated* value from elsewhere in
// the runtime (observed as a Symbol) - a stronger signal that some other
// frame's data was aliased in, not just a dropped write.
func TestIssue247ReproLarger(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		class EventStream {
			queue = [];
			waiting = [];
			done = false;
			push(event) {
				if (this.done) return;
				const waiter = this.waiting.shift();
				if (waiter) {
					waiter({ value: event, done: false });
				} else {
					this.queue.push(event);
				}
			}
			end() {
				this.done = true;
				while (this.waiting.length > 0) {
					const waiter = this.waiting.shift();
					waiter({ value: undefined, done: true });
				}
			}
			async *[Symbol.asyncIterator]() {
				while (true) {
					if (this.queue.length > 0) {
						yield this.queue.shift();
					} else if (this.done) {
						return;
					} else {
						const result = await new Promise((resolve) => this.waiting.push(resolve));
						if (result.done) return;
						yield result.value;
					}
				}
			}
		}

		async function* fakeChunks(n) {
			for (let i = 0; i < n; i++) {
				await new Promise((r) => setTimeout(r, 0));
				yield { delta: { reasoning_content: "x" + i } };
			}
		}

		var brokenCount = 0;
		function startStreaming(n) {
			const stream = new EventStream();
			(async () => {
				const output = { content: [] };
				let thinkingBlock = null;
				const blocks = output.content;
				const getContentIndex = (block) => {
					if (!Array.isArray(blocks) || typeof blocks.indexOf !== "function") {
						brokenCount++;
						return -999;
					}
					return blocks.indexOf(block);
				};
				for await (const chunk of fakeChunks(n)) {
					if (chunk.delta.reasoning_content) {
						if (!thinkingBlock) {
							thinkingBlock = { type: "thinking", thinking: "" };
							blocks.push(thinkingBlock);
						}
						thinkingBlock.thinking += chunk.delta.reasoning_content;
						const idx = getContentIndex(thinkingBlock);
						stream.push({ type: "thinking_delta", contentIndex: idx, partial: output });
					}
				}
				stream.push({ type: "done", message: output });
				stream.end();
			})();
			return stream;
		}

		var eventCount = 0;
		async function main() {
			const stream = startStreaming(80);
			for await (const event of stream) eventCount++;
		}
		main();
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}

	broken, ok := p.GetVM().GetGlobal("brokenCount")
	if !ok {
		t.Fatal("brokenCount not found")
	}
	if broken.ToFloat() != 0 {
		t.Errorf("expected brokenCount === 0 (blocks must stay the live array throughout), got %s", broken.Inspect())
	}
	eventCount, ok := p.GetVM().GetGlobal("eventCount")
	if !ok {
		t.Fatal("eventCount not found")
	}
	if eventCount.ToFloat() != 81 {
		t.Errorf("expected 81 events processed (80 chunks + done), got %s", eventCount.Inspect())
	}
}

// TestAsyncClosureWritableWhileParked exercises the *suspend-side* half of
// the fix specifically: every other test above only reads/writes the
// captured binding after the suspended function resumes. This one calls the
// escaped closure while the async function is still parked at its await -
// the write must land in the save buffer relocateOpenUpvalues points the
// upvalue at, and resume must pick it up via its own copy+relocate.
func TestAsyncClosureWritableWhileParked(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		let probe = null;
		let result = null;
		async function f() {
			let n = 1;
			probe = () => { n++; };
			await Promise.resolve(); // always suspends per spec, even for a settled promise
			return n;
		}
		f().then((r) => { result = r; });
		// f is parked here - probe() must mutate the paused function's own binding,
		// not a stale copy or (if the bug were still present) some unrelated
		// register-stack slot.
		probe();
		probe();
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	result, ok := p.GetVM().GetGlobal("result")
	if !ok {
		t.Fatal("result not found")
	}
	if result.ToFloat() != 3 {
		t.Errorf("expected 3 (1 + two probe() calls while parked), got %s", result.Inspect())
	}
}

// TestConcurrentGeneratorInstancesDontCrossContaminate covers the
// "concurrency" the issue's title actually names: two independently
// suspended instances of the *same* generator function, resumed in an
// interleaved order, must each keep their own closure's view of their own
// captured local - relocateOpenUpvalues must never point one instance's
// upvalue at another instance's save buffer.
func TestConcurrentGeneratorInstancesDontCrossContaminate(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
		function* g() {
			let n = 1;
			const get = () => n;
			yield get;
			n = 100;
			yield null;
			n = n + 1;
			yield null;
		}
		let a = g();
		let b = g();
		const getA = a.next().value;
		const getB = b.next().value;
		a.next(); // a: n=100, b untouched
		let snap1 = [getA(), getB()];
		b.next(); // b: n=100
		let snap2 = [getA(), getB()];
		a.next(); // a: n=101
		let snap3 = [getA(), getB()];
		[snap1, snap2, snap3]
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !result.IsArray() {
		t.Fatalf("expected array result, got %s", result.Inspect())
	}
	arr := result.AsArray()
	if arr.Length() != 3 {
		t.Fatalf("expected 3 snapshots, got %d", arr.Length())
	}
	want := [3][2]string{{"100", "1"}, {"100", "100"}, {"101", "100"}}
	for i, w := range want {
		snap := arr.Get(i)
		if !snap.IsArray() || snap.AsArray().Length() != 2 {
			t.Fatalf("snapshot %d: expected a 2-element array, got %s", i, snap.Inspect())
		}
		got := [2]string{snap.AsArray().Get(0).ToString(), snap.AsArray().Get(1).ToString()}
		if got != w {
			t.Errorf("snapshot %d: got %v, want %v", i, got, w)
		}
	}
}

// spillPadding returns n throwaway `let` declarations that are never freed
// (all live in the same top-level function scope for its whole body), to
// push the compiler's register allocator for that function past
// VariableRegisterThreshold (200, see pkg/compiler/regalloc.go) so that
// whatever local is declared after them lands in a spill slot
// (frame.spillSlots) rather than a register.
func spillPadding(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("let pad")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(" = 0;\n")
	}
	return b.String()
}

// TestAsyncClosureMutationAfterResumeStaysCoherentWithSpilledLocal is the
// spill-slot counterpart of TestAsyncClosureMutationAfterResumeStaysCoherent:
// the captured local `n` is forced into a spill slot (not a register) by 250
// throwaway padding declarations ahead of it, then mutated both directly and
// via a closure after resume. Note this alone does not reproduce the
// frame.spillSlots restore bug (with nothing else running between suspend
// and resume, the same CallFrame slot happens to still hold this call's own
// spillSlots array even without an explicit restore) - it's a coherence
// check, kept alongside the true discriminator below
// (TestConcurrentAsyncFunctionsWithSpilledLocalsDontCrossContaminate, which
// forces the reused-slot collision and does fail without the fix).
func TestAsyncClosureMutationAfterResumeStaysCoherentWithSpilledLocal(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		async function f() {
			` + spillPadding(250) + `
			let n = 1; // forced into a spill slot by the 250 padding locals above
			const get = () => n;
			const inc = () => { n++; };
			await new Promise((r) => setTimeout(r, 0));
			inc();
			n = n + 10;
			return [n, get()];
		}
		let result = null;
		f().then((r) => { result = r; });
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}

	result, ok := p.GetVM().GetGlobal("result")
	if !ok || !result.IsArray() {
		t.Fatalf("expected array result, got %v", result)
	}
	arr := result.AsArray()
	if arr.Length() != 2 || arr.Get(0).ToString() != "12" || arr.Get(1).ToString() != "12" {
		t.Errorf("expected [12, 12] (direct write and closure read of the spilled local must agree), got %s", result.Inspect())
	}
}

// TestConcurrentAsyncFunctionsWithSpilledLocalsDontCrossContaminate runs two
// concurrently-suspended instances of the same async function, each with a
// spilled captured local, resumed together off the same timer tick - each
// instance's closure must keep observing its own spill array, never the
// other instance's.
//
// (Deliberately uses the *same* delay for both: an earlier version of this
// test gave the two instances different delays and hit an unrelated,
// pre-existing timer-ordering bug where the earlier-scheduled instance's
// post-await continuation silently never ran its own body - confirmed with
// no spilling involved at all, so out of scope here. Same delay sidesteps it
// without weakening what this test is actually checking.)
func TestConcurrentAsyncFunctionsWithSpilledLocalsDontCrossContaminate(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		async function f(tag) {
			` + spillPadding(250) + `
			let n = 1; // forced into a spill slot
			const get = () => n;
			await new Promise((r) => setTimeout(r, 1));
			n = n + 100;
			return { tag, value: get() };
		}
		let results = [];
		Promise.all([f("a"), f("b")]).then((rs) => { results = rs; });
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}

	results, ok := p.GetVM().GetGlobal("results")
	if !ok || !results.IsArray() || results.AsArray().Length() != 2 {
		t.Fatalf("expected a 2-element results array, got %v", results)
	}
	arr := results.AsArray()
	seen := map[string]string{}
	for i := 0; i < 2; i++ {
		entry := arr.Get(i).AsPlainObject()
		tag, _ := entry.GetOwn("tag")
		value, _ := entry.GetOwn("value")
		seen[tag.ToString()] = value.ToString()
	}
	if seen["a"] != "101" || seen["b"] != "101" {
		t.Errorf("expected both instances' spilled local to independently reach 101, got %v", seen)
	}
}

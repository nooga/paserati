// expect: true
// no-typecheck

// #142: a promise reaction handler that throws must not leak VM-internal
// "still unwinding" state past the point where its rejection has been fully
// absorbed by the promise machinery.
//
// triggerPromiseReactions (pkg/vm/promise.go) calls vm.Call(reaction.Handler,
// ...) to run a .then()/.catch() handler. When the handler throws, vm.Call
// hands the exception back as a Go error, which triggerPromiseReactions
// turns into a rejected promise (reaction.Reject(reason)) instead of
// re-throwing - the exception has been fully handled at that point. But the
// VM-internal executeUserFunctionSafe/executeUserFunctionWithNewTarget that
// produced that Go error used to leave vm.unwinding=true even after handing
// the exception off, on the theory that whatever received it would either
// make a new vm.Call (which clears the stale flag on entry) or re-throw
// (which sets it fresh regardless). Absorbing the exception into a promise
// rejection is a third case neither of those cover: nothing clears the
// flag, and vm.unwinding is checked bare after nearly every VM instruction
// (that's the actual convention - a fresh, per-operation flag, not sticky
// state), including once per instruction in the main dispatch loop. Left
// true, the very next instruction any vm.run() call executes - anywhere,
// possibly much later - sees a phantom "still unwinding" with no actual
// exception value and either aborts outright or corrupts unrelated control
// flow.
//
// This reproduces it with two promises: `leaked`'s reaction throws and gets
// absorbed by triggerPromiseReactions (the leak), scheduled to run AFTER
// `marker`'s own resolution reaction in the same microtask drain pass, so
// nothing else clears the leaked flag before `await marker` resumes. Before
// the fix, this aborted the whole script with a bogus
// "Uncaught exception: null" instead of continuing normally.
//
// PRECONDITION this test depends on and does not itself enforce: `leaked`'s
// throwing reaction must run strictly AFTER `marker`'s resolution reaction
// within the same microtask drain pass triggered by `await marker` below.
// That ordering is what "nothing else clears the leaked flag before `await
// marker` resumes" means concretely - reverse the order (or a future change
// to reaction scheduling / RunUntilIdle's batching reorders them) and this
// test passes trivially even with the fix reverted, because
// executeUserFunctionSafe's entry guard would scrub the leaked flag when
// `marker`'s own resolution reaction runs afterward instead.
//
// WHAT THIS TEST ACTUALLY DISCRIMINATES - measured, not assumed. The #142
// fix landed in two places, and either one alone is sufficient for THIS
// scenario, so the test is an OR over both:
//
//   vm_init.go clear only      -> PASSES
//   promise.go clear only      -> PASSES
//   both reverted              -> FAILS, "Uncaught exception: null"
//
// So this test proves the leak was real and that it is now closed on the
// promise-reaction path, but it does NOT pin down the general fix on its
// own: reverting just the four `vm.unwinding = false` lines in
// pkg/vm/vm_init.go leaves it green, because triggerPromiseReactions'
// own ClearUnwindingState() call absorbs the leak one level up. Do not
// read a green run here as coverage of executeUserFunctionSafe /
// executeUserFunctionWithNewTarget in isolation.
//
// The general clear in vm_init.go is kept because the promise path is only
// one absorber shape: any native code that takes a vm.Call exception as a
// Go error, absorbs it, and returns nil without re-throwing or making
// another vm.Call hits the same trap, and vm_init.go is the only place that
// closes it for all of them. It is not covered by this smoke test, but it
// is far from untested - it is worth +21 Test262 built-ins tests on its
// own, all in exception handling (Array.from iterator-close errors,
// Promise.all capability-resolve throws, Iterator.zip suspended-yield
// close, Reflect.apply/construct argument-list errors, TypedArray Get key
// errors, RegExp Symbol.search / String matchAll throws). Reverting the
// four lines in vm_init.go gives all 21 back up, which is the check to run
// if anyone is tempted to narrow this to the promise path alone.
//
// Two things the general clear also does, both deliberate:
//
//  - It exposed one real latent bug and it is fixed alongside it: vm.go's
//    isUnscopable() documented its contract as "if hadError, check
//    vm.unwinding", which only worked while vm.Call left the flag set. Its
//    vm.GetProperty error path now re-throws instead. See the comment
//    there; language/statements/with/unscopables-prop-get-err.js covers it.
//
//  - It briefly turned language/types/reference/put-value-prop-base-primitive-realm.js
//    from a pass into a failure. That test is a FALSE PASS both before and
//    after this PR, so read no signal into it either way. On main the leaked
//    flag aborts the script at its cross-realm `other.eval(...)` before the
//    first assertion runs, and the runner scores a silently aborted script as
//    a pass (paserati#65). Closing the leak let it reach the assertion and
//    fail honestly; the primitive-base setter fix in op_setprop.go then made
//    the trap's own exception propagate, which aborts the script at the same
//    `other.eval(...)` again - so it is back to "passing" by abort. Verified
//    with print() markers: the script still never reaches its first
//    assertion.
//
//    Two separate, pre-existing gaps keep it there, neither touched here:
//    (1) a cross-realm call does not switch to the callee's [[Realm]], so the
//    main-realm set trap throws ReferenceError on its own `numberCount`
//    global when invoked from the other realm; and (2) an exception escaping
//    `realmGlobal.eval(...)` silently aborts the script instead of being
//    catchable - confirmed pre-existing by running a plain
//    `try { other.eval('throw new Error("x")') } catch (e) {}` on a
//    a88125c2 worktree, where it also aborts uncatchably.
//
// If this test ever stops catching the both-reverted failure, it no longer
// exercises #142 at all and needs rewriting (e.g. re-establishing the
// ordering some other way), not deleting.

let resolveMarker: (v: number) => void;
const marker = new Promise<number>((res) => {
  resolveMarker = res;
});
Promise.resolve().then(() => resolveMarker(42));

// Scheduled after the above, so its throwing reaction runs LAST in the same
// microtask drain pass, after `marker` has already settled - leaving no
// further vm.Call to scrub the leaked state before `await marker` resumes.
const leaked = Promise.resolve(1).then(() => {
  throw new Error("leak");
});

const result = await marker;

// A genuinely new, unrelated exception right after resuming must still be
// caught normally instead of the resumption itself spuriously aborting.
let caught = false;
try {
  throw new Error("real throw after leak");
} catch (e) {
  caught = e instanceof Error && e.message === "real throw after leak";
}

// Silence "unhandled rejection" noise / unused-var lint for `leaked` without
// otherwise touching it (do NOT await it - the whole point is that nothing
// consumes it again before the assertions above run).
void leaked;

result === 42 && caught;

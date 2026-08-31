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
// `marker`'s own resolution reaction runs afterward instead. Verified by
// hand at the time this was written: reverting the vm_init.go fix makes
// this test fail with exactly "Uncaught exception: null". If this test ever
// stops catching that failure, it no longer exercises #142 and needs
// rewriting (e.g. re-establishing the ordering some other way), not
// deleting.

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

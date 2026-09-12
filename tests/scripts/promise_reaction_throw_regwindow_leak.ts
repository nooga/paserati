// expect: true
// #414: a promise reaction whose JS handler throws is invoked via
// triggerPromiseReactions's vm.Call(reaction.Handler, ...) (pkg/vm/
// promise.go); when the callee throws, that vm.Call returns the exception
// as a Go error (via executeUserFunctionSafe), which the reaction absorbs
// into a rejection instead of re-throwing. executeUserFunctionSafe's (and
// executeUserFunctionWithNewTarget's) error paths dropped the frame(s)
// unwinding stopped at via truncateFramesTo, but - like Interpret's own
// nested-call error path already knew to guard against - truncateFramesTo
// deliberately doesn't reclaim the register directory's cursor for those
// frames (see its own doc comment, #61). Left unreclaimed, that call's
// whole register window stayed allocated above whatever frame resumes
// next - typically the top-level script frame itself, resuming from an
// unrelated `await` - which is exactly the shape this test pins: it
// doesn't need to observe the leaked promise at all, only that a LATER,
// unrelated await still works and the top-level frame's own register
// window is left in the state it started in.
//
// Under plain execution this always ran fine and produced the correct
// value; only this suite's strict register-window checks
// (vm.StrictRegWindowChecks, tests/scripts_test.go's TestMain) turn the
// leak into a hard panic at the top-level frame's eventual pop
// (popTopLevelScriptFrame -> checkRegWindowRelease), which is exactly why
// this needs a smoke test rather than being "harmless in production".
const p = Promise.resolve(undefined).then(() => {
  throw new Error("leak");
});
// Deliberately never awaited/caught - the leak happens the moment this
// reaction runs and throws, regardless of whether anything ever consumes
// the resulting rejection.
void p;

const marker = await Promise.resolve(42);

marker === 42;

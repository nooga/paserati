// expect: true
// #414: the constructor-call sibling of promise_reaction_throw_regwindow_leak.ts.
// Reflect.construct(ctor, args) is the one built-in path that reaches
// executeUserFunctionWithNewTarget (vm.ConstructWithNewTarget, per
// pkg/builtins/reflect_init.go) from native Go code, exactly the way a
// promise reaction's handler reaches executeUserFunctionSafe via vm.Call.
// When ctor's body throws, that call's error path used to drop the frame(s)
// via truncateFramesTo without reclaiming the register directory's cursor
// (see truncateFramesTo's own doc comment) - the same leak as the plain
// vm.Call case, just through executeUserFunctionWithNewTarget instead of
// executeUserFunctionSafe. Verified by temporarily reverting the
// vm.regDir.popTo(entryRegMark) calls added to executeUserFunctionWithNewTarget
// (pkg/vm/vm_init.go) and confirming this test fails the same way.
class Thrower {
  constructor() {
    throw new Error("ctor leak");
  }
}

const p = Promise.resolve(undefined).then(() => {
  Reflect.construct(Thrower, []);
});
// Deliberately never awaited/caught - see the sibling test's comment for why
// that doesn't matter: the leak happens the moment this reaction runs.
void p;

const marker = await Promise.resolve(42);

marker === 42;

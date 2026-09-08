// expect: ReferenceError:true:true:after:undefined:true
// no-typecheck
// Same bug as eval_indirect_runtime_error_catchable.ts, but for direct eval
// (OpDirectEval / DirectEvalCode), which shares the vm.Interpret nested-run
// codepath but has its own separate error-to-exception wiring in vm.go's
// OpDirectEval handling.
function run(): string {
  let caughtName = "no-throw";
  let isReferenceError = false;
  try {
    eval("undefinedVarInDirectEval;");
  } catch (e) {
    caughtName = e.name;
    isReferenceError =
      e instanceof ReferenceError &&
      e.message === "undefinedVarInDirectEval is not defined";
  }

  let ranAfterCatch = false;
  let afterMarker = "before";
  try {
    eval("(0, 0);");
    ranAfterCatch = true;
    afterMarker = "after";
  } catch (e) {
    afterMarker = "unexpected-throw";
  }

  // A thrown `undefined` must come through as-is, not get miscategorized as
  // "no wrapped exception" (see OpDirectEval's gotExc handling) and
  // rewrapped as a SyntaxError.
  let undefThrowType = "no-throw";
  let undefThrowIsUndefined = false;
  try {
    eval("throw undefined;");
  } catch (e) {
    undefThrowType = typeof e;
    undefThrowIsUndefined = e === undefined;
  }

  return (
    caughtName +
    ":" +
    isReferenceError +
    ":" +
    ranAfterCatch +
    ":" +
    afterMarker +
    ":" +
    undefThrowType +
    ":" +
    undefThrowIsUndefined
  );
}

run();

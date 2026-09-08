// expect: ReferenceError:true:after
// no-typecheck
// A `with` block reading an identifier that exists neither on the with-object
// nor at global scope must raise a normal, catchable ReferenceError - not an
// uncatchable internal error that aborts the whole script. The OpGetWithProperty
// handler used to call vm.runtimeError(...) directly for this "not found
// anywhere" fallback case, which appends to vm.errors and returns
// InterpretRuntimeError without going through vm.throwException/unwindException,
// so no surrounding try/catch (even one directly wrapping the with block) could
// ever catch it.
let caughtName = "no-throw";
let isReferenceError = false;
try {
  with ({}) {
    undefinedInsideWithXYZ;
  }
} catch (e) {
  caughtName = e.name;
  isReferenceError =
    e instanceof ReferenceError &&
    e.message === "undefinedInsideWithXYZ is not defined";
}

// Execution must continue normally after the catch.
let afterMarker = "before";
afterMarker = "after";

caughtName + ":" + isReferenceError + ":" + afterMarker;

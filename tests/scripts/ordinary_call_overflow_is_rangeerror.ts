// no-typecheck
// Regression test for #407: stack overflow from ordinary (non-`new`)
// recursive function calls must throw a catchable RangeError with the
// spec-correct "Maximum call stack size exceeded" message, matching the
// `new`-expression overflow path (see ctor_stack_overflow_catchable.ts),
// not a generic uncatchable-as-RangeError Error.
//
// Also pins down the actual bug #407 reported: naively converting the
// overflow to a RangeError inside prepareCall itself broke resumption after
// the throw entirely (the catch block below never ran, script exited with
// no output at all) whenever the immediate caller had no handler of its own
// and the unwind had to cross several plain bytecode frames with nothing
// native in between - exactly what happens here.
// expect: RangeError:Maximum call stack size exceeded
function f() {
  f();
}

function callback() {
  f(); // no handler of its own - the catch below is one frame further out
}

let result = "";
try {
  callback();
} catch (e) {
  result = e.constructor.name + ":" + e.message;
}
result;

// expect: true
// paserati#426: a `new Function(...)` body large/deeply-nested enough to hit
// an internal compiler limit (this synthetic shape - sequential nested
// `if`s - mirrors real generated code from JSON-Schema validators, bundled
// SDK clients, etc.) must surface as a normal, catchable SyntaxError from
// the `new Function()` call site, never crash the process or leak past user
// code as an unrecoverable panic. See also compileDynamicFunctionSource in
// pkg/builtins/function_init.go, which wraps the parse+compile of
// dynamically-constructed function bodies in its own recover() specifically
// so an internal compiler panic (not just a clean, already-returned-as-an-
// error limit) is contained the same way instead of escaping as a raw Go
// panic.
//
// Depth 2000 (rather than the issue's original 150) deliberately targets
// the *next* limit past register exhaustion - a separate, pre-existing,
// already-gracefully-handled "function too large: ... exceeds the 16-bit
// branch limit" bytecode-size error - because paserati#426's own nested-if
// *register* leak (conditionReg/consequenceReg held live for an entire
// nested chain instead of freed once dead) was since fixed, raising the
// register-exhaustion ceiling from ~118 to over 1000 nested levels. 150
// (or even 1000) no longer errors at all once that leak is fixed, which
// would make this test's own premise stale - see the compiler package's
// nested-if/ternary register-liveness fix.
let body = "let x = 0;\n";
let close = "";
for (let i = 0; i < 2000; i++) {
  body += "if (data.a" + i + " !== undefined) { x += " + i + ";\n";
  close += "}\n";
}
body += close + "return x;";

let caught = false;
try {
  new Function("data", body);
} catch (e) {
  caught = e instanceof SyntaxError;
}

caught;

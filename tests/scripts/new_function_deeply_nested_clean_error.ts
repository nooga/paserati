// expect: true
// paserati#426: a `new Function(...)` body large/deeply-nested enough to hit
// the compiler's internal register-allocator limit (this synthetic shape -
// 150 sequential nested `if`s - mirrors real generated code from JSON-Schema
// validators, bundled SDK clients, etc.) must surface as a normal, catchable
// SyntaxError from the `new Function()` call site, never crash the process
// or leak past user code as an unrecoverable panic. See also
// compileDynamicFunctionSource in pkg/builtins/function_init.go, which wraps
// the parse+compile of dynamically-constructed function bodies in its own
// recover() specifically so an internal compiler panic (not just the clean
// "register exhaustion" error already returned here) is contained the same
// way instead of escaping as a raw Go panic.
let body = "let x = 0;\n";
let close = "";
for (let i = 0; i < 150; i++) {
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

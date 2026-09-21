// expect: true
// paserati#426: a `new Function(...)` body large/deeply-nested enough to hit
// an internal compiler limit (this synthetic shape - sequential nested
// `if`s - mirrors real generated code from JSON-Schema validators, bundled
// SDK clients, etc.) must never crash the process or leak past user code as
// an unrecoverable panic. See also compileDynamicFunctionSource in
// pkg/builtins/function_init.go, which wraps the parse+compile of
// dynamically-constructed function bodies in its own recover() specifically
// so an internal compiler panic is contained the same way a clean,
// already-returned-as-an-error limit would be.
//
// Depth 2000 (rather than the issue's original 150) deliberately targets
// the *next* limit past register exhaustion - a separate "function too
// large: ... exceeds the ... branch limit" bytecode-size error - because
// paserati#426's own nested-if *register* leak (conditionReg/consequenceReg
// held live for an entire nested chain instead of freed once dead) was since
// fixed, raising the register-exhaustion ceiling from ~118 to over 1000
// nested levels.
//
// This depth used to hit that next limit too: paserati#482 found a real,
// large ajv-generated validator function whose jump distances exceeded the
// bytecode format's old 16-bit signed branch offset (±32767 bytes), which
// this same nested-if shape reproduces at depth 2000. #482's fix widened
// jump/branch offsets to 32 bits, so this depth no longer hits *any* known
// compiler limit - it now compiles and runs correctly, which this test
// verifies directly instead of expecting a graceful compile error.
let depth = 2000;
let body = "let x = 0;\n";
let close = "";
let expected = 0;
let obj: Record<string, number> = {};
for (let i = 0; i < depth; i++) {
  body += "if (data.a" + i + " !== undefined) { x += " + i + ";\n";
  close += "}\n";
  obj["a" + i] = 1;
  expected += i;
}
body += close + "return x;";

let f = new Function("data", body);
f(obj) === expected;

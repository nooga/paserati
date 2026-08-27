// no-typecheck
// Test that let/const declared inside eval() get their own fresh lexical
// environment with proper TDZ (Temporal Dead Zone) tracking, per ECMAScript
// EvalDeclarationInstantiation - referencing the name before its declaration
// line, even from within the eval string itself, must throw a ReferenceError.
// The binding must also stay local to the eval call: it must never leak into
// the calling scope.
// expect: eval TDZ checks passed

let letTdzThrew = false;
try {
  eval("typeof x; let x;");
} catch (e) {
  letTdzThrew = e instanceof ReferenceError;
}
if (!letTdzThrew) throw new Error("expected ReferenceError for TDZ access to eval-local let");

let constTdzThrew = false;
try {
  eval("typeof y; const y = 1;");
} catch (e) {
  constTdzThrew = e instanceof ReferenceError;
}
if (!constTdzThrew) throw new Error("expected ReferenceError for TDZ access to eval-local const");

// Normal (post-declaration) access still works.
if (eval("let z = 5; z + 1;") !== 6) throw new Error("normal eval-local let usage regressed");

// A closure created inside the eval must capture the eval-local binding itself.
if (eval("let a = 1; (function() { return a; })();") !== 1)
  throw new Error("closure over eval-local let regressed");

// The eval's own let/const must shadow an identically-named binding from the
// calling scope, not defer to it (per EvalDeclarationInstantiation, eval gets
// its own fresh lexical environment).
function shadowsCallerVar() {
  var x = 1;
  let threw = false;
  try {
    eval("typeof x; let x;");
  } catch (e) {
    threw = e instanceof ReferenceError;
  }
  return threw && x === 1;
}
if (!shadowsCallerVar()) throw new Error("eval-local let did not shadow caller's var binding");

function shadowsCallerLet() {
  let x = 1;
  let threw = false;
  try {
    eval("typeof x; let x;");
  } catch (e) {
    threw = e instanceof ReferenceError;
  }
  return threw && x === 1;
}
if (!shadowsCallerLet()) throw new Error("eval-local let did not shadow caller's let binding");

// The binding must not leak outside the eval call.
eval("let leaked = 1;");
let leakThrew = false;
try {
  // @ts-ignore - intentionally referencing a name that must not exist here
  leaked;
} catch (e) {
  leakThrew = e instanceof ReferenceError;
}
if (!leakThrew) throw new Error("eval-local let leaked into the calling scope");

// const reassignment inside eval still throws.
let constReassignThrew = false;
try {
  eval("const w = 1; w = 2;");
} catch (e) {
  constReassignThrew = e instanceof TypeError;
}
if (!constReassignThrew) throw new Error("expected TypeError reassigning eval-local const");

"eval TDZ checks passed";

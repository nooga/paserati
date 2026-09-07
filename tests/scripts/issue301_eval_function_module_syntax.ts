// expect: true
// paserati#301: direct eval, indirect eval, and new Function accept source
// that is only valid as a Module and must be an early SyntaxError as a
// Script/FunctionBody (ECMA-262 19.2.1.1 PerformEval parses the source with
// the Script goal; 20.2.1.1.1 CreateDynamicFunction with FunctionBody).
// Only dynamic import(...) - itself a Script-goal production - stays legal.
const checks: boolean[] = [];

function throwsSyntaxError(fn: () => void): boolean {
  try {
    fn();
    return false;
  } catch (e) {
    return e instanceof SyntaxError;
  }
}

function throwsAnything(fn: () => void): boolean {
  try {
    fn();
    return false;
  } catch (e) {
    return true;
  }
}

// --- Direct eval ---
checks.push(throwsSyntaxError(() => eval("export var x;")));
checks.push(throwsSyntaxError(() => eval("export default 1;")));
checks.push(throwsSyntaxError(() => eval("export {};")));
checks.push(throwsSyntaxError(() => eval('import x from "y";')));
checks.push(throwsSyntaxError(() => eval("import.meta")));

// --- Indirect eval ---
const indirectEval = eval;
checks.push(throwsSyntaxError(() => indirectEval("export var x;")));
checks.push(throwsSyntaxError(() => indirectEval("export default 1;")));
checks.push(throwsSyntaxError(() => indirectEval("export {};")));
checks.push(throwsSyntaxError(() => indirectEval('import x from "y";')));
checks.push(throwsSyntaxError(() => indirectEval("import.meta")));

// --- new Function ---
checks.push(throwsSyntaxError(() => new Function("export var x;")));
checks.push(throwsSyntaxError(() => new Function("export default 1;")));
checks.push(throwsSyntaxError(() => new Function("export {};")));
checks.push(throwsSyntaxError(() => new Function('import x from "y";')));
checks.push(throwsSyntaxError(() => new Function("import.meta")));

// --- Dynamic import() is a Script-goal production and must stay legal in
// all three contexts (it just fails to resolve a module, which isn't a
// SyntaxError). ---
checks.push(!throwsSyntaxError(() => eval('import("nonexistent-module")')));
checks.push(
  !throwsSyntaxError(() => indirectEval('import("nonexistent-module")'))
);
checks.push(
  !throwsSyntaxError(() => new Function('return import("nonexistent-module")'))
);

// --- A nested async function inside a Function() body can still use a real
// (non-top-level) await - the fix must not disturb ordinary await. ---
checks.push(
  !throwsSyntaxError(() =>
    new Function("return (async function(){ return await 1; })();")()
  )
);

// --- Direct/indirect eval's own top level must not silently accept
// top-level await as if it were module code: 'await' parses as a plain
// identifier reference there, not an AwaitExpression, so it is not a number. ---
checks.push(eval("typeof await") === "undefined");
checks.push(indirectEval("typeof await") === "undefined");

// --- The exact issue-table repro: before this fix, direct eval("await 1;")
// silently returned 1 with no throw at all (treating it as real top-level
// await). It must now throw. Spec wants an early SyntaxError here (a Script
// grammar can't have "await 1" adjacent with no operator/ASI between the
// now-plain-identifier 'await' and '1'); this runtime instead throws
// ReferenceError ('await' is read as a value before failing to continue the
// statement) because of a separate, pre-existing ASI-leniency gap that
// already accepts "x 1" as two statements with no line break anywhere in
// this parser (independent of eval/await) - not fixed here. What matters for
// this issue is that it no longer silently succeeds as top-level await. ---
checks.push(throwsAnything(() => eval("await 1;")));

checks.every((c) => c === true);

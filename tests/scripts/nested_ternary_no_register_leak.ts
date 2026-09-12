// expect: true
// paserati#426 Symptom 1 companion: compileTernaryExpression had the same
// condition-register leak as compileIfExpression (freed only at function
// return via a deferred cleanup, so a chain of nested ternaries pinned one
// register per level for the whole nested chain even though each one is
// dead immediately after the conditional jump consumes it). Verifies a
// 500-level nested ternary compiles and evaluates correctly instead of
// hitting "register exhaustion: expression too deeply nested".
let expr = "0";
for (let i = 0; i < 500; i++) {
  expr = "(data.a" + i + " !== undefined ? " + i + " : (" + expr + "))";
}
const fn = new Function("data", "return " + expr + ";");

fn({}) === 0 && fn({ a3: true }) === 3 && fn({ a499: true }) === 499;

// expect: TDZ 'a'|TDZ 'b'|TDZ 'c'
// Every declarator of a let/const clause needs its Temporal Dead Zone marker,
// not just the first. Four pre-registration sites read the statement's legacy
// first-declaration alias (s.Name) instead of its declarator list, so for
// `let a = 1, b = 2` only `a` was marked. Reading `b` before the declaration
// then fell through to an unbound global and reported
//
//   ReferenceError: b is not defined
//
// where node reports "Cannot access 'b' before initialization".
//
// This file covers the top-level script path, whose TDZ error names the
// variable. The block, function-body and direct-eval paths are in
// tdz_every_declarator_nested.ts, which has to skip type checking: the checker
// rejects a use-before-declaration inside a block or function outright with
// TS2304 rather than tsc's TS2448, a separate pre-existing gap that is the same
// for every declarator.

let out: string[] = [];

// Classifies the failure so a "not defined" regression is unmistakable.
function classify(e: any, name: string): string {
  const msg = String(e.message);
  if (msg.indexOf("before initialization") < 0) return "NOT-TDZ: " + msg;
  if (msg.indexOf("'" + name + "'") >= 0) return "TDZ '" + name + "'";
  return "TDZ (unnamed)";
}

try {
  a;
} catch (e) {
  out.push(classify(e, "a"));
}
try {
  b;
} catch (e) {
  out.push(classify(e, "b"));
}
try {
  c;
} catch (e) {
  out.push(classify(e, "c"));
}

let a = 1,
  b = 2,
  c = 3;
void a;
void b;
void c;

out.join("|");

// expect: 28
// paserati#220: function declarations named after a TS contextual keyword
// (satisfies, is, infer, readonly, override, abstract, keyof) failed to
// parse with "'(' expected" - parseFunctionLiteral's hand-maintained list
// of tokens allowed as a function name was missing all seven. Real Node
// accepts all of them as ordinary function names/identifiers since none of
// these are reserved words at the JS runtime level.
function satisfies(e: number): number {
  return e;
}
function is(e: number): number {
  return e;
}
function infer(e: number): number {
  return e;
}
function readonly(e: number): number {
  return e;
}
function override(e: number): number {
  return e;
}
function abstract(e: number): number {
  return e;
}
function keyof(e: number): number {
  return e;
}
satisfies(1) + is(2) + infer(3) + readonly(4) + override(5) + abstract(6) + keyof(7);

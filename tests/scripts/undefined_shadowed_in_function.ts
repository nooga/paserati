// expect: 99
// paserati#440: `undefined` is not a reserved word, so it can be shadowed by
// a local binding - including under a function scope with full type checking
// enabled (the global scope pre-declares `undefined` itself, so a top-level
// `let undefined = ...` collides with that and is a separate, legitimate
// redeclaration error; a nested scope has no such conflict). The checker's
// visit(*parser.UndefinedLiteral) case unconditionally typed every read as
// `undefined`, ignoring the shadowing local variable entirely.
function f(): number {
  let undefined = 99;
  return undefined;
}
f();

// skip-typecheck
// expect: 42
// paserati#440: `undefined` is not a reserved word in ECMAScript, just an
// ordinary identifier - so it's legal to shadow it with a local binding, and
// real Node does so correctly:
//   let undefined = 42; console.log(undefined, typeof undefined) -> "42 number"
// The lexer gives `undefined` its own dedicated token, and the parser's
// prefix-parse function for that token unconditionally produced an
// UndefinedLiteral AST node in expression position, regardless of any local
// binding - so every read of the shadowed name still resolved to the literal
// undefined value instead of the shadowing variable.
let undefined = 42;
console.log(typeof undefined);
undefined;

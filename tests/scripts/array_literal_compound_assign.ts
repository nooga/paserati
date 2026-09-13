// paserati#454 - a compound assignment (x += 1, obj.y -= 2, ...) as an
// array-literal element used to fail with "expected ',' or ']' in array
// literal" - only plain `=` was handled, since parseArrayLiteral's
// destructuring-default cover grammar only special-cased ASSIGN. Compound
// operators have no destructuring-default meaning, so they should just
// parse as ordinary assignment expressions.
// expect: 5,2,5
// no-typecheck

let a = 5, b = 1, c = null;
[a -= 0, b *= 2, c ||= 5].join(",");

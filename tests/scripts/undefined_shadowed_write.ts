// skip-typecheck
// expect: 43
// paserati#440 follow-up: writes to a shadowed `undefined` were still
// inert - `undefined = value` didn't even parse (isValidLValue rejected
// UndefinedLiteral as an assignment target), so the read-side fix alone
// didn't cover reassignment of the shadowing local:
//   let undefined = 42; undefined = 43; undefined -> 43 in real Node.
let undefined = 42;
undefined = 43;
undefined;

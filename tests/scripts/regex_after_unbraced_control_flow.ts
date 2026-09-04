// paserati#235: a regex literal right after an if/while/for/with header's
// closing ')' - with no braces around the body - used to misparse as
// division, since RPAREN isn't in the lexer's canBeRegexStart set (correctly,
// for the common case where ')' closes a value-producing expression) and the
// parser never re-lexed the peeked token in regex context for this specific
// boundary, unlike every other statement-boundary token it handles.
// expect: true

let hits = 0;

// if with un-braced body
if (true) /x/.test("x") && hits++;

// while with un-braced body
let n = 0;
while (n++ < 1) /x/.test("x") && hits++;

// for with un-braced body
for (let i = 0; i < 1; i++) /x/.test("x") && hits++;

// for-of / for-in with un-braced bodies
for (const _ of [1]) /x/.test("x") && hits++;
for (const _ in { a: 1 }) /x/.test("x") && hits++;

// with with un-braced body (non-strict only - `with` throws in strict mode)
with ({ a: 1 }) /x/.test("x") && hits++;

// Division after a value-producing parenthesized expression must still work
// (not misdetected as a regex start just because a `for`/`if`/etc. is nearby).
let divided = (4 + 2) / 3;

hits === 6 && divided === 2;

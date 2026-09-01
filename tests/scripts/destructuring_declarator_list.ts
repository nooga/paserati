// expect: 1,2|1,2,3|1,2|1,2|10,20|10,undefined|10,20|10,20|10,20|1,2|1,2|1,2,3|1,[2,3],9|1,{"b":2},9|5,1|7,1
// A binding pattern is legal in ANY declarator position, not just the first.
// Both halves of that were broken:
//
//   #159  `let r = 1, {a} = obj` failed to parse outright. Each of
//         parseLetStatement/parseConstStatement/parseVarStatement carried its
//         own copy of the declarator loop, and every copy only accepted an
//         identifier after a comma.
//
//   #160  `let {a} = obj, b = 2` parsed but silently destructured the wrong
//         value. The destructuring declaration's initializer was parsed at
//         LOWEST, so it swallowed `, b = 2` as a comma operator: the pattern's
//         source became `(obj, b = 2)`, i.e. b's own value. Reported as a
//         register-lifetime bug; the register was fine, the parse wasn't.
//         An Initializer is an AssignmentExpression, so it must never consume
//         the comma separating declarators - COMMA precedence, as the
//         identifier path already used.
//
// All three keywords now share one declarator-list parser
// (parseVariableDeclarationList). A clause that mixes plain bindings with
// patterns desugars to one declaration statement per declarator, grouped in a
// DeclarationGroup that introduces no scope of its own.

let out: string[] = [];
function rec(...vals: any[]) {
  out.push(vals.map((v) => JSON.stringify(v)).join(","));
}

// --- #159: pattern as a non-first declarator ---
let r1 = 1,
  { a: h1 } = { a: 2 };
rec(r1, h1);

let r2 = 1,
  [a2, b2] = [2, 3];
rec(r2, a2, b2);

const r3 = 1,
  { a: h3 } = { a: 2 };
rec(r3, h3);

var r4 = 1,
  { a: h4 } = { a: 2 };
rec(r4, h4);

// --- #160: pattern as the first of several declarators ---
let { a: a5 } = { a: 10 },
  b5 = 20;
rec(a5, b5);

// The no-initializer form threw "ReferenceError: b is not defined", because the
// swallowed `, b` was parsed as a *read* of an undeclared global.
let { a: a6 } = { a: 10 },
  b6;
rec(a6, b6);

const { a: a7 } = { a: 10 },
  b7 = 20;
rec(a7, b7);

var { a: a8 } = { a: 10 },
  b8 = 20;
rec(a8, b8);

let [a9] = [10],
  b9 = 20;
rec(a9, b9);

// --- several patterns in one clause ---
let { a: a10 } = { a: 1 },
  { b: b10 } = { b: 2 };
rec(a10, b10);

let [p11] = [1],
  [q11] = [2];
rec(p11, q11);

// A pattern in the middle, with plain bindings on both sides.
let a12 = 1,
  { b: b12 } = { b: 2 },
  c12 = 3;
rec(a12, b12, c12);

// --- pattern features still work in a list: rest, nesting, defaults ---
let [a13, ...rest13] = [1, 2, 3],
  z13 = 9;
rec(a13, rest13, z13);

let { a: a14, ...rest14 } = { a: 1, b: 2 },
  z14 = 9;
rec(a14, rest14, z14);

let {
    a: { b: b15 },
  } = { a: { b: 5 } },
  z15 = 1;
rec(b15, z15);

let { a: a16 = 7 } = {},
  z16 = 1;
rec(a16, z16);

out.join("|");

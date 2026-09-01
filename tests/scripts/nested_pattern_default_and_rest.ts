// expect: 7|1|3|5|4|9|8,[9,10]|1,[2,3]|s,[true]|2|1,5|1,2,3|4|6|11|12|13,[14,15]
// A default value or rest element *nested inside* a destructuring pattern was
// rejected by the type checker, though the runtime bound both correctly:
//
//   const {h: {i = 7}} = {h: {}};      // invalid destructuring target type: *parser.AssignmentExpression
//   const [[j, ...k]] = [[8, 9, 10]];  // invalid destructuring target type: *parser.SpreadElement
//
// while a top-level default (`{e = 5}`) and a top-level rest (`[j, ...k]`) both
// checked fine, as did nested patterns with neither.
//
// A pattern is represented two ways depending on depth. At the top level of a
// declaration a default lives in DestructuringProperty.Default /
// DestructuringElement.Default and a rest in DestructuringElement.IsRest, with
// Target holding the binding. Nested one level down, Target is a raw
// ObjectLiteral/ArrayLiteral, so a default appears as an AssignmentExpression
// and a rest as a SpreadElement, with the binding inside the wrapper. The four
// destructuring-target walks knew Identifier/ObjectLiteral/ArrayLiteral but
// neither wrapper.

let out: string[] = [];

// --- nested defaults, taken and not taken ---
const {
  h: { i = 7 },
} = { h: {} };
out.push(String(i));

const {
  h2: { i2 = 7 },
} = { h2: { i2: 1 } };
out.push(String(i2));

const {
  a: { b: { c = 3 } },
} = { a: { b: {} } };
out.push(String(c));

const { a2: [q = 5] } = { a2: [] };
out.push(String(q));

const [[m = 4]] = [[]];
out.push(String(m));

const [[m2 = 4]] = [[9]];
out.push(String(m2));

// --- nested rest elements ---
const [[j, ...k]] = [[8, 9, 10]];
out.push(j + "," + JSON.stringify(k));

const {
  r: [r1, ...rrest],
} = { r: [1, 2, 3] };
out.push(r1 + "," + JSON.stringify(rrest));

// A rest binding over a nested TUPLE gets the union of the remaining member
// types, not just the type at its own position - otherwise `string[]` would be
// silently accepted for a value holding a boolean. Matches the top-level case.
const tup: [[string, boolean]] = [["s", true]];
const [[t1, ...trest]] = tup;
const trestOk: boolean[] = trest;
out.push(t1 + "," + JSON.stringify(trestOk));

// --- nested defaults and rest in a for-of head. This walk defines names
// without validating them, so the missing wrapper cases made it bind nothing
// and the later read was "Cannot find name" rather than a target-type error.
let forOfOut = 0;
for (const {
  f: { g = 2 },
} of [{ f: {} }]) {
  forOfOut = g;
}
out.push(String(forOfOut));

let forOfRest = "";
for (const [[x, ...ys]] of [[[1, 5]]]) {
  forOfRest = x + "," + ys[0];
}
out.push(forOfRest);

let forOfDeep = "";
for (const { p: [a1 = 1, ...rest1] } of [{ p: [] as number[] }]) {
  forOfDeep = [a1, 2, 3].join(",");
  void rest1;
}
out.push(forOfDeep);

// --- the same nested shapes in a for INITIALIZER ---
let forInitOut = 0;
for (const [[fi = 4]] = [[]]; forInitOut === 0; ) {
  forInitOut = fi;
}
out.push(String(forInitOut));

// --- and in a destructuring ASSIGNMENT, which uses two further target walks ---
let asgA = 0;
({
  z: { asgA = 6 },
} = { z: {} });
out.push(String(asgA));

let asgB = 0;
({ y: [asgB = 11] } = { y: [] });
out.push(String(asgB));

let asgC = 0;
[[asgC = 12]] = [[]];
out.push(String(asgC));

let asgD = 0;
let asgRest: number[] = [];
[[asgD, ...asgRest]] = [[13, 14, 15]];
out.push(asgD + "," + JSON.stringify(asgRest));

out.join("|");

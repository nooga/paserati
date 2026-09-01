// expect: 0,1,2|0,1,2|0,1|ok|3,2,1|undefined,undefined,1,2
// A `for` initializer is a declarator list too, so a binding pattern is legal
// in any of its positions: `for (let i = 0, {a} = obj; ...)`. It shared the
// same defects as statement position (#159, #160) and the same fix - see
// destructuring_declarator_list.ts.
//
// A for initializer must be a single Statement, so unlike statement position
// (where the desugared declarations are spliced into the enclosing statement
// list) the DeclarationGroup survives here, and the checker and compiler
// process its members in order without opening a scope. `var` hoisting has to
// see through the group as well.

let out: string[] = [];

// #159 order: plain binding first, pattern second.
let acc1: number[] = [];
for (let i = 0, { a } = { a: 3 }; i < a; i++) acc1.push(i);
out.push(acc1.join(","));

// #160 order: pattern first. Previously `a` was undefined, so `i < a` was false
// and the loop body never ran.
let acc2: number[] = [];
for (let { a } = { a: 3 }, i = 0; i < a; i++) acc2.push(i);
out.push(acc2.join(","));

// var, and an array pattern.
let acc3: number[] = [];
for (var i3 = 0, [x3] = [2]; i3 < x3; i3++) acc3.push(i3);
out.push(acc3.join(","));

// const in a for head needs an initializer on every declarator.
for (const { a: a4 } = { a: 1 }, b4 = 2; false; ) {
}
out.push("ok");

// A lone pattern in the head still works (it always did).
let acc5: number[] = [];
for (let { a: a5 } = { a: 3 }; a5 > 0; a5--) acc5.push(a5);
out.push(acc5.join(","));

// var declarations in the head hoist to the top of the function, through the
// group: v6 and w6 both exist (as undefined) before the loop runs, and both are
// readable after it. The pattern half used to report TS2304 - the checker
// defined a var pattern's bindings in the current scope rather than the
// function scope - which is why this assertion originally covered only v6.
function hoisted(): string {
  const before = [
    typeof v6 === "undefined" ? "undefined" : "defined",
    typeof w6 === "undefined" ? "undefined" : "defined",
  ];
  for (var v6 = 1, { w: w6 } = { w: 2 }; false; ) {
  }
  return [before[0], before[1], v6, w6].join(",");
}
out.push(hoisted());

out.join("|");

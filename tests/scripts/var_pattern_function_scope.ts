// expect: 1|2|3|4|5|six|51|52,53|3,3,3|1,2,3|1,2,3|1,2,3|1,2,3|5,2,5|1/[9],2/[8]|1,2,3|1,2|1,3|1/10 1/20 2/10 2/20|99|99|99|99|99|99|13
// `var` is function-scoped, so its bindings belong in the nearest function (or
// global) scope no matter how many blocks deep the declaration sits. The checker
// has no var-hoisting pre-pass - it defines a binding when it visits the
// declaration - and the plain VarStatement path routes that through
// GetFunctionScope, but the two *destructuring* target walkers defined into
// c.env, the current scope. So a var pattern one level deeper than the function
// body vanished at the end of its block:
//
//   function f(){ { var {w} = {w: 2}; } return w; }   // TS2304: Cannot find name 'w'.
//
// while both `var {w} = ...` directly in the body and `{ var w = 2; }` resolved
// fine. Same for a pattern in a for-of or for-init head, in a switch case, a try
// block, a while body, or a labeled block.
//
// Every case below asserts the *value*, which also proves the name resolves.
// This test grew in three passes, each one converting deferred assertions into
// real ones as the matching half landed: the checker scope fix, the declaration
// write-through fix, and the loop-head write-through fix. Nothing is deferred
// now.

let out: string[] = [];

// --- a pattern in a for-of head: name resolves AND the value is right ---
function forOfHead(): number {
  for (var { w } of [{ w: 1 }]) {
  }
  return w;
}
out.push(String(forOfHead()));

function forOfHeadArray(): number {
  for (var [w] of [[2]]) {
  }
  return w;
}
out.push(String(forOfHeadArray()));

// --- a pattern in a for-init head, alone and mixed with a plain declarator ---
function forInitHead(): number {
  for (var { w } = { w: 3 }; false; ) {
  }
  return w;
}
out.push(String(forInitHead()));

function forInitHeadMixed(): number {
  for (var v = 9, { w } = { w: 4 }; false; ) {
  }
  return w;
}
out.push(String(forInitHeadMixed()));

// --- a var declared in a `for` HEAD nested one block deeper. The head
// pre-declared its name in the *immediately* enclosing scope - the block, not
// the function - so it reported TS2304 before the scope fix, and then wrote to
// that shadow rather than the hoisted binding: a plain binding read back
// undefined and a pattern threw ReferenceError, because the loop scope holding
// the shadow was popped before the read.
function forOfInBlock(): number {
  {
    for (var q of [5]) {
    }
  }
  return q;
}
out.push(String(forOfInBlock()));

function forInInBlock(): string {
  {
    for (var k in { six: 1 }) {
    }
  }
  return k;
}
out.push(forInInBlock());

function forInitInBlock(): number {
  {
    for (var fi = 51; false; ) {
    }
  }
  return fi;
}
out.push(String(forInitInBlock()));

// A pattern in a for-of head inside a block threw ReferenceError: its names were
// never hoisted at all - collectVarDeclarations only recognised a plain
// VarStatement in a loop head, not a destructuring declaration.
function forOfPatternInBlock(): string {
  {
    for (var { w } of [{ w: 52 }]) {
    }
  }
  {
    for (var [z] of [[53]]) {
    }
  }
  return [w, z].join(",");
}
out.push(forOfPatternInBlock());

// var in a loop head is ONE binding, reassigned each iteration, so closures over
// it all see the last value - unlike a let head, which is per-iteration.
function varHeadIsOneBinding(): string {
  const fs: (() => number)[] = [];
  for (var vh of [1, 2, 3]) {
    fs.push(() => vh);
  }
  return fs.map((g) => g()).join(",");
}
out.push(varHeadIsOneBinding());

function letHeadIsPerIteration(): string {
  const fs: (() => number)[] = [];
  for (let lh of [1, 2, 3]) {
    fs.push(() => lh);
  }
  return fs.map((g) => g()).join(",");
}
out.push(letHeadIsPerIteration());

// The same, for a let/const *pattern* head. These used to give the last value
// for every closure: only the plain-identifier branches of the head dispatch
// recorded their register as a per-iteration binding, so the pattern branches -
// whose names are bound deep inside the destructuring helpers - were never
// closed at the loop's back edge.
function letPatternHeadIsPerIteration(): string {
  const fs: (() => number)[] = [];
  for (let { w } of [{ w: 1 }, { w: 2 }, { w: 3 }]) {
    fs.push(() => w);
  }
  return fs.map((g) => g()).join(",");
}
out.push(letPatternHeadIsPerIteration());

function constPatternHeadIsPerIteration(): string {
  const fs: (() => number)[] = [];
  for (const [w] of [[1], [2], [3]]) {
    fs.push(() => w);
  }
  return fs.map((g) => g()).join(",");
}
out.push(constPatternHeadIsPerIteration());

// A NESTED target is the discriminating shape: it is bound through
// compileNestedPatternDeclaration rather than the head's own binding code, so it
// only works if the per-iteration collection looks names up after the whole
// pattern is bound.
function nestedPatternHeadIsPerIteration(): string {
  const fs: (() => number)[] = [];
  for (let {
    a: { b },
  } of [{ a: { b: 1 } }, { a: { b: 2 } }, { a: { b: 3 } }]) {
    fs.push(() => b);
  }
  return fs.map((g) => g()).join(",");
}
out.push(nestedPatternHeadIsPerIteration());

// Defaults and rest elements reach their bindings through yet other paths.
function defaultAndRestHeadsArePerIteration(): string {
  const withDefault: (() => number)[] = [];
  for (let { a = 5 } of [{}, { a: 2 }, {}]) {
    withDefault.push(() => a);
  }
  const withRest: (() => string)[] = [];
  for (let [a, ...r] of [
    [1, 9],
    [2, 8],
  ]) {
    withRest.push(() => a + "/" + JSON.stringify(r));
  }
  return (
    withDefault.map((g) => g()).join(",") +
    "|" +
    withRest.map((g) => g()).join(",")
  );
}
out.push(defaultAndRestHeadsArePerIteration());

// A let/const pattern in a C-style for INITIALIZER is a per-iteration binding
// too: the spec copies the bindings before the first test and after each update.
function letPatternForInitIsPerIteration(): string {
  const fs: (() => number)[] = [];
  for (let { w } = { w: 1 }; w < 4; w++) {
    fs.push(() => w);
  }
  return fs.map((g) => g()).join(",");
}
out.push(letPatternForInitIsPerIteration());

// break and continue must not disturb the captures.
function patternHeadWithBreak(): string {
  const fs: (() => number)[] = [];
  for (let { w } of [{ w: 1 }, { w: 2 }, { w: 3 }]) {
    fs.push(() => w);
    if (w === 2) break;
  }
  return fs.map((g) => g()).join(",");
}
out.push(patternHeadWithBreak());

function patternHeadWithContinue(): string {
  const fs: (() => number)[] = [];
  for (let { w } of [{ w: 1 }, { w: 2 }, { w: 3 }]) {
    if (w === 2) continue;
    fs.push(() => w);
  }
  return fs.map((g) => g()).join(",");
}
out.push(patternHeadWithContinue());

// Nested loops each get their own per-iteration bindings.
function nestedPatternLoops(): string {
  const fs: (() => string)[] = [];
  for (let { w } of [{ w: 1 }, { w: 2 }]) {
    for (let { v } of [{ v: 10 }, { v: 20 }]) {
      fs.push(() => w + "/" + v);
    }
  }
  return fs.map((g) => g()).join(" ");
}
out.push(nestedPatternLoops());

// --- the shapes that reach the function scope through an enclosing statement.
// Each was TS2304 (a compile error failing the whole file) before the scope fix,
// and read back undefined before the write-through fix.
function inBlock(): number {
  {
    var { w } = { w: 99 };
  }
  return w;
}
out.push(String(inBlock()));

function inIf(): number {
  if (true) {
    var { w } = { w: 99 };
  }
  return w;
}
out.push(String(inIf()));

function inSwitch(): number {
  switch (1) {
    case 1:
      var { w } = { w: 99 };
  }
  return w;
}
out.push(String(inSwitch()));

function inTry(): number {
  try {
    var { w } = { w: 99 };
  } catch (e) {}
  return w;
}
out.push(String(inTry()));

function inWhile(): number {
  while (true) {
    var { w } = { w: 99 };
    break;
  }
  return w;
}
out.push(String(inWhile()));

function inLabeled(): number {
  lbl: {
    var { w } = { w: 99 };
  }
  return w;
}
out.push(String(inLabeled()));

// --- the same at script top level, where the "function scope" is the global
// env. The for-of head's runtime is correct here, so assert the value.
for (var { top } of [{ top: 13 }]) {
}
out.push(String(top));

out.join("|");

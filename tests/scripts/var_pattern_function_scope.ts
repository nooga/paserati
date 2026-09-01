// expect: 1|2|3|4|5|6|7|8|9|10|11|12|13
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
// WHAT THIS TEST ASSERTS: that the *name resolves*. It deliberately does not
// assert the destructured value for the cases below that go through an
// enclosing block, because a separate open codegen bug makes `var` with a
// pattern inside a block write a block-local binding instead of the hoisted one,
// so those read back as undefined at function scope - identically with
// --no-typecheck, i.e. nothing to do with the checker. Those cases just
// reference the name (`void w`), which is what fails to compile when the name
// does not resolve. The for-of and for-init heads are the shapes whose runtime
// is already correct, so those assert real values.

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

// --- a plain var in a for-of/for-in head nested one block deeper. The head
// hoisted its name to the *immediately* enclosing scope, which is the block, not
// the function - so this reported TS2304 for a plain binding too.
function forOfInBlock(): string {
  {
    for (var q of [1]) {
    }
  }
  void q;
  return "5";
}
out.push(forOfInBlock());

function forInInBlock(): string {
  {
    for (var k in { a: 1 }) {
    }
  }
  void k;
  return "6";
}
out.push(forInInBlock());

// --- the shapes that reach the function scope through an enclosing statement.
// Reading `w` after the enclosing statement IS the assertion: before the fix
// each of these was TS2304, a compile error that failed the whole file. The
// value is deliberately not asserted - see the header note - so `void w` records
// the reference without depending on what it holds.
function inBlock(): string {
  {
    var { w } = { w: 99 };
  }
  void w;
  return "7";
}
out.push(inBlock());

function inIf(): string {
  if (true) {
    var { w } = { w: 99 };
  }
  void w;
  return "8";
}
out.push(inIf());

function inSwitch(): string {
  switch (1) {
    case 1:
      var { w } = { w: 99 };
  }
  void w;
  return "9";
}
out.push(inSwitch());

function inTry(): string {
  try {
    var { w } = { w: 99 };
  } catch (e) {}
  void w;
  return "10";
}
out.push(inTry());

function inWhile(): string {
  while (false) {
    var { w } = { w: 99 };
  }
  void w;
  return "11";
}
out.push(inWhile());

function inLabeled(): string {
  lbl: {
    var { w } = { w: 99 };
  }
  void w;
  return "12";
}
out.push(inLabeled());

// --- the same at script top level, where the "function scope" is the global
// env. The for-of head's runtime is correct here, so assert the value.
for (var { top } of [{ top: 13 }]) {
}
out.push(String(top));

out.join("|");

// expect: 1|2|3|4|5|6|99|99|99|99|99|99|13
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
// Every case below asserts the destructured *value*, which also proves the name
// resolves. The block-family cases originally asserted only resolution: a
// companion codegen bug made a `var` pattern inside a block write a block-local
// binding instead of the hoisted one, so they read back as undefined. Both
// halves are fixed - see the commit that repaired the write-through.

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
//
// These two assert resolution only. A `var` declared in a `for` *head* inside a
// nested block still writes a block-local rather than the hoisted binding - a
// separate open bug in the loop-head path, broken identically for a plain
// binding (`{ for (var q of [5]) {} } q` reads undefined) and worse for a
// pattern (`{ for (var {w} of xs) {} } w` throws ReferenceError). Unrelated to
// the declaration path this test covers; asserting the value here would bake in
// the wrong number.
function forOfInBlock(): string {
  {
    for (var q of [5]) {
    }
  }
  void q;
  return "5";
}
out.push(forOfInBlock());

function forInInBlock(): string {
  {
    for (var k in { six: 1 }) {
    }
  }
  void k;
  return "6";
}
out.push(forInInBlock());

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

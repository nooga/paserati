// expect: true
// skip-typecheck
// paserati#182: spreading the arguments object (fn(...arguments),
// [...arguments], super(...arguments)) threw "TypeError: object is not
// iterable", even though arguments is genuinely iterable by every other
// means (for-of, Array.from, destructuring). extractSpreadArguments' fast
// paths didn't have a TypeArguments case, and its generic fallback only
// recognized Symbol.iterator on a hand-picked list of prototype-walkable
// types that never included TypeArguments (whose Symbol.iterator is a
// real own property, not reachable via a prototype chain at all).
//
// Spreading `arguments` is itself rejected by the type checker separately
// (its declared type isn't recognized as spreadable) - unrelated to this
// issue, which is specifically about the *runtime* behavior real,
// checker-bypassing JS/TS hosts (plain paserati's own --no-typecheck flag,
// and embeddings like noderati) hit - hence skip-typecheck above.
const checks = [];

function basicSpread() {
  return [...arguments];
}
checks.push(JSON.stringify(basicSpread(1, 2, 3)) === "[1,2,3]");

function callSpread(a, b, c) {
  return a + b + c;
}
function callSpreadArgs() {
  return callSpread(...arguments);
}
checks.push(callSpreadArgs(10, 20, 30) === 60);

// The exact real-world shape this issue's impact section calls out:
// super(...arguments) in a pass-through derived-class constructor, where
// the base constructor has a rest parameter. This also exercises a
// second, independent bug found while verifying the fix's real-world
// impact: handleOpSpreadNew (used for both super(...)/new Foo(...) with a
// spread argument) never handled a variadic callee's rest parameter at
// all - it copied spread arguments positionally into registers, so a
// `...args` rest parameter received the raw first argument (or undefined)
// instead of a real array.
class Base {
  constructor(...args) {
    this.args = args;
  }
}
class PassThrough extends Base {
  constructor() {
    super(...arguments);
  }
}
const pt = new PassThrough(1, 2, 3);
checks.push(JSON.stringify(pt.args) === "[1,2,3]");

// A third bug found alongside the rest-parameter one: handleOpSpreadNew
// never padded registers beyond the spread argument count with Undefined,
// so a declared parameter past what was actually spread kept whatever
// stale value happened to already be sitting in that register-stack slot
// from an earlier, unrelated frame - reachable only when something else
// dirties the same register-stack range first.
function dirtyRegisters(a, b, c, d) {
  return a + b + c + d;
}
dirtyRegisters("W", "X", "Y", "Z");
class Fixed4 {
  constructor(a, b, c, d) {
    this.a = a;
    this.b = b;
    this.c = c;
    this.d = d;
  }
}
class FewerArgs extends Fixed4 {
  constructor() {
    super(...[1, 2]);
  }
}
const fa = new FewerArgs();
checks.push(fa.a === 1 && fa.b === 2 && fa.c === undefined && fa.d === undefined);

// handleOpSpreadNew backs BOTH super(...) and plain `new Foo(...)` - the two
// bugs above were fixed at that shared call site, so verify the non-super
// direction too rather than assuming super() coverage implies it.
dirtyRegisters("W", "X", "Y", "Z");
const plainNew = new Fixed4(...[1, 2]);
checks.push(plainNew.a === 1 && plainNew.b === 2 && plainNew.c === undefined && plainNew.d === undefined);
class PlainRest {
  constructor(...r) {
    this.r = r;
  }
}
checks.push(JSON.stringify(new PlainRest(...[1, 2, 3]).r) === "[1,2,3]");

// arguments.length is itself a writable own property (op_setprop.go stores
// a reassignment as a namedProps override rather than touching the raw
// length field) - spread must read the current, possibly-reassigned
// length the same way Array.from(arguments) already does, not the raw
// original argument count.
function reassignedLength() {
  arguments.length = 1;
  return [...arguments];
}
checks.push(JSON.stringify(reassignedLength(1, 2, 3)) === "[1]");

checks.every((c) => c === true);

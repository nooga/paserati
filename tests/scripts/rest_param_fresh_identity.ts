// Regression test for paserati roadmap item A4: an empty rest parameter
// used to be the same mutable shared array object on every call, so
// mutating one caller's rest array leaked into every other call's (and its
// length stayed wrong afterward). Every call must now get a fresh,
// independently identifiable empty array. Covers direct, spread, bound,
// constructor, generator, and tail-call routes. Async functions are known
// to still return `undefined` instead of the rest array at all when
// awaited (a separate, pre-existing bug unrelated to this fix - see the
// spawned "Fix rest params in awaited async functions" task), so the async
// route is intentionally left out here rather than asserted against.
// expect: true

function direct(...args: any[]) {
  return args;
}

let d1 = direct();
let d2 = direct();
d1.push(42);
let directOk = d1 !== d2 && d2.length === 0 && d1.length === 1;

function callSpread(fn: (...a: any[]) => any) {
  return fn(...([] as any[]));
}
let s1 = callSpread(direct);
let s2 = callSpread(direct);
s1.push(1);
let spreadOk = s1 !== s2 && s2.length === 0;

let bound = direct.bind(null);
let b1 = bound();
let b2 = bound();
b1.push(1);
let boundOk = b1 !== b2 && b2.length === 0;

class C {
  args: any[];
  constructor(...args: any[]) {
    this.args = args;
  }
}
let c1 = new C();
let c2 = new C();
c1.args.push(1);
let ctorOk = c1.args !== c2.args && c2.args.length === 0;

function tail(n: number, ...args: any[]): any[] {
  if (n <= 0) return args;
  return tail(n - 1);
}
let t1 = tail(3);
let t2 = tail(3);
t1.push(1);
let tailOk = t1 !== t2 && t2.length === 0;

function* gen(...args: any[]) {
  yield args;
}
let g1 = gen().next().value as any[];
let g2 = gen().next().value as any[];
g1.push(1);
let genOk = g1 !== g2 && g2.length === 0;

directOk && spreadOk && boundOk && ctorOk && tailOk && genOk;

// expect: 2
// Trailing parameters whose type accepts void may be omitted by callers.
function createDeferred<TValue = void>() {
  let resolve!: (value: TValue) => void;
  const promise = new Promise<TValue>((res) => { resolve = res; });
  return { promise, resolve };
}
const d = createDeferred();
d.resolve();
let calls = 0;
const f = (v: void) => { calls++; };
f();
const g = (a: number, v: void | undefined) => { calls += a; };
g(1);
class C { constructor(public v: void) {} }
new C();
calls;

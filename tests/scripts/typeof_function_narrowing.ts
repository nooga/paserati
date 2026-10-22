// expect: 6
const o: { any?: (s: number[]) => number } = { any: (s) => s.length };
let total = 0;
if (typeof o.any === 'function') { total += o.any([1, 2]); }
const u: unknown = (a: number, b: number) => a + b;
if (typeof u === 'function') { total += u(1, 2); }
function call<T>(x: T): number { return typeof x === 'function' ? x(1) : 0; }
total += call((n: number) => n);
const p: any = { f: () => 0 };
if (typeof p === 'object' && typeof p.f === 'function') { total += p.f(5); }
total;

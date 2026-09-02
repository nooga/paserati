// Optional calls (?.()) whose callee is a member expression must support
// spread arguments, not just literal ones (issue #185... #188).
const o: any = { f: (a: number, b: number) => a + b };
const args = [3, 4];

// obj.method?.(...args)
const r1 = o.f?.(...args);

// obj["method"]?.(...args)
const r2 = o["f"]?.(...args);

// obj[computedKey]?.(...args) -- dynamic index callee
const o2: any = { 2: (a: number, b: number) => ["two", a, b] };
const r3 = o2[args.length]?.(...args);

// Wrapped in ?? : must call through, not silently become undefined.
const r4 = o2[args.length]?.(...args) ?? "fallback";

// Nullish member callee still short-circuits correctly with spread present.
const n: any = null;
const r5 = n?.f?.(...args);

// a?.b?.(...) - optional chaining base callee with spread.
const o3: any = { inner: { g: (a: number, b: number) => a - b } };
const r6 = o3?.inner?.g?.(...[10, 3]);

// super.method?.(...) with spread.
class Base {
  m(a: number, b: number): number {
    return a * b;
  }
}
class Derived extends Base {
  m2(a: number, b: number): number | undefined {
    return super.m?.(...[a, b]);
  }
}
const r7 = new Derived().m2(3, 4);

JSON.stringify([r1, r2, r3, r4, r5, r6, r7]);
// expect: [7,7,["two",3,4],["two",3,4],null,7,12]

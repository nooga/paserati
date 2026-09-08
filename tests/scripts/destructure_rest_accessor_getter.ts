// Array destructuring's rest element (`const [x, ...rest] = arr`, and the
// assignment-pattern form `[x, ...rest] = arr`) must honor an own accessor
// property (get/set installed via Object.defineProperty) on the source array
// exactly like plain property access already does - the rest binding always
// goes through the generic iterator protocol (a rest element disables the
// array-destructuring fast path, see destructure_array_fast_path.ts), whose
// element reads previously bypassed accessors (found while testing PR
// "fix(vm): array spread must call an own accessor's getter, not read raw
// storage"). Covers: a plain getter, a setter-only accessor (reads as
// undefined), a throwing getter (propagates as a catchable exception), a
// getter that shrinks the array's own backing storage mid-rest (must not
// panic), the assignment-pattern form, the plain no-accessor path
// (regression guard), and for-of over the same kind of array (same shared
// iterator state backs both). Values are read back via JSON.stringify
// (rather than Array.prototype.join, which has its own unrelated bug
// rendering undefined/null elements as the literal word instead of empty -
// out of scope here) and cross-checked against Node.
// expect: [42,3]|[2,null]|caught:boom|[99]|[7,3]|[2,3]|[1,99,3]
let out: string[] = [];

// 1. Plain getter at an in-bounds index
const a = [1, 2, 3];
Object.defineProperty(a, "1", { get() { return 42; }, enumerable: true, configurable: true });
const [, ...restA] = a;
out.push(JSON.stringify(restA));

// 2. Setter-only accessor reads as undefined (JSON.stringify renders it as null)
const b = [1, 2, 3];
Object.defineProperty(b, "2", { set(_v: number) {}, enumerable: true, configurable: true });
const [, ...restB] = b;
out.push(JSON.stringify(restB));

// 3. A throwing getter propagates as a catchable exception
const c = [1, 2, 3];
Object.defineProperty(c, "1", { get() { throw new Error("boom"); }, enumerable: true, configurable: true });
try {
  const [, ...restC] = c;
  out.push("no throw:" + JSON.stringify(restC));
} catch (e: any) {
  out.push("caught:" + e.message);
}

// 4. A getter that shrinks the array's own backing storage mid-rest must not
// panic - Length() and the raw-element fallback are both re-read live, so
// the rest simply stops early (matches Node).
const d = [1, 2, 3, 4, 5];
Object.defineProperty(d, "1", { get() { (d as any).length = 0; return 99; }, enumerable: true, configurable: true });
const [, ...restD] = d;
out.push(JSON.stringify(restD));

// 5. Assignment-pattern rest (not a declaration)
let p: number, restE: number[];
const e = [1, 2, 3];
Object.defineProperty(e, "1", { get() { return 7; }, enumerable: true, configurable: true });
[p, ...restE] = e;
out.push(JSON.stringify(restE));

// 6. No-accessor path is unaffected (regression guard)
const f = [1, 2, 3];
const [, ...restF] = f;
out.push(JSON.stringify(restF));

// 7. for-of over the same kind of array (blast radius: the array's default
// iterator is shared between for-of and rest destructuring)
const g = [1, 2, 3];
Object.defineProperty(g, "1", { get() { return 99; }, enumerable: true, configurable: true });
let forOfOut: number[] = [];
for (const v of g) forOfOut.push(v);
out.push(JSON.stringify(forOfOut));

out.join("|");

// expect: 3
// A script's interface named like a built-in class merges into it.
interface Date {
  extra(): number;
}
declare const d: Date;
function f(x: Date): number {
  return x.getTime() > 0 ? 1 : 0;
}
// x! and unknown-typed variables are assumed initialized (no TS2454).
let resolve: (v: number) => void;
new Promise<number>((res) => {
  resolve = res;
});
const r = resolve!;
let u: unknown;
[1].forEach(() => {
  u = 2;
});
const z = u;
f(new Date()) + (z as number);

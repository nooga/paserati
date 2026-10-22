// expect: 5
declare function fail(): never;
function f(x: string | number): number {
  if (typeof x === "string") { fail(); }
  return x;
}
5;

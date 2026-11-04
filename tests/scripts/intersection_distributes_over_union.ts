// expect: 7
// An intersection with a union distributes over it (A & (B | C) is
// (A & B) | (A & C)), so a discriminant narrows it and the shared members
// destructure.
type Op = { id: number; path: string };
type Opts = Op & ({ direction: "down"; elapsedMs: number } | { direction: "up" });
function f(opts: Opts & { colorMode: "none" | "css" }): number {
  const { direction, path } = opts;
  if (direction === "up") { return path.length; }
  return opts.elapsedMs;
}
interface X { a?: { aProp: string }; b?: { bProp: string }; }
type AorB = { a: object; b: undefined } | { a: undefined; b: object };
declare const q: X & AorB;
function g(): string {
  if (q.a !== undefined) { return q.a.aProp; }
  return q.b.bProp;
}
f({ id: 1, path: "abc", direction: "up", colorMode: "css" }) + f({ id: 1, path: "", direction: "down", elapsedMs: 4, colorMode: "none" });

// expect: 2
type K = "a" | "b";
function f(k: K): number {
  switch (k) {
    case "a": return 1;
    case "b": return 2;
    default: { const n: never = k; return n; }
  }
}
function rawr(d: K): string {
  if (d === "a") return "x";
  if (d === "b") return "y";
  throw "Unexpected " + d;
}
f("b");

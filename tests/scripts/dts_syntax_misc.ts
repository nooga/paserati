// Declaration-file syntax: keyword-named type aliases, a leading `&`, and an
// overload signature ended by a line break instead of `;`.
// expect: 3
type override<b> = b;
type X =
  & { a: 1 }
  & { b: 2 };
function f(a: string): number
function f(a: any) { return 1; }
const x: X = { a: 1, b: 2 };
const o: override<number> = 2;
f("a") + o;

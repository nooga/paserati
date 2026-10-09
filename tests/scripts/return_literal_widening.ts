// expect: 2
function f() { return { a: 1, b: "x" }; }
type R = ReturnType<typeof f>;
const r: R = { a: 2, b: "y" };
r.a;

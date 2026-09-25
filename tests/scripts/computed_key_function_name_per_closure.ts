// SetFunctionName from a computed key applies to each closure, not to the
// shared function template (#563).
// no-typecheck
// expect: a,b,x,y,m1,m2,named,false
function mk(n) { return { [n]: function () {} }[n]; }
function mk2(n) { return { [n]: () => 1 }[n]; }
function mk3(n) { return { [n]() {} }[n]; }
function mk4(n) { return { [n]: function named() {} }[n]; }
const d = Object.getOwnPropertyDescriptor(mk("z"), "name");
[mk("a").name, mk("b").name, mk2("x").name, mk2("y").name, mk3("m1").name, mk3("m2").name, mk4("q").name, d.writable].join(",");

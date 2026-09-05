// expect: 42|2|3|foo,bar,baz,q|120
// #265: an anonymous function assigned to an object-literal property (or to a
// variable) gets its .name inferred from the key, but that inferred name must NOT
// become a self-binding inside the function body - a closed-over outer variable
// with the same name has to win.
function make(sync: (x: number) => number) {
  return { sync: function (x: number) { return sync(x); } };
}
const a = make((x) => x * 2).sync(21);

// Same shape, plain assignment: the body must see the (reassigned) outer f.
var f: any = function () { return f; };
var g = f;
f = 2;
const b = g();

// ...and logical assignment.
var h: any;
h ||= function () { return h; };
var k = h;
h = 3;
const c = k();

// Name inference itself is unchanged.
const o: any = { foo: function () {}, bar: function () {}, baz: () => {} };
let q: any;
q = function () {};
const names = [o.foo.name, o.bar.name, o.baz.name, q.name].join(",");

// A genuinely named function expression as a property still binds its own name.
const o2 = { fact: function fact(n: number): number { return n <= 1 ? 1 : n * fact(n - 1); } };
const d = o2.fact(5);

[a, b, c, names, d].join("|");

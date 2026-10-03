// expect: 8|8|2.5|42|u,u,u,u|foo|3|u
// no-typecheck
// paserati#587: a typed array element read through a string or BigInt key
// returned undefined. A canonical numeric key is an element read (10.4.5.4),
// answered from the elements alone - an invalid index is undefined even when
// the prototype has that key - while other keys still use the prototype.
const u = new Uint8Array([7, 8, 9]);
const f = new Float64Array([1.5, 2.5]);
u[2n] = 42;

const proto = Object.create(Uint8Array.prototype);
proto["5"] = "proto";
proto["-0"] = "proto";
proto["1.5"] = "proto";
proto["01"] = "u";
proto.foo = "foo";
Object.setPrototypeOf(u, proto);

const show = (v) => (v === undefined ? "u" : String(v));
[
  u["1"],
  u[1n],
  f[1n],
  u["2"],
  [u["5"], u["-0"], u["1.5"], u[5]].map(show).join(","),
  u["foo"],
  u["length"],
  u["01"],
].join("|");

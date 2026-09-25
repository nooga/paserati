// An ordinary object whose prototype is an array inherits the array's
// elements and length: get, in, Reflect.has, for-in (#571).
// no-typecheck
// expect: 2,1,2,F|true,true,false,true|own,0,1,nm|2|1,9,true
const a = [1, 2];
a.nm = "F";
const o = Object.create(a);
o.own = 3;
const keys = [];
for (const k in o) keys.push(k);
[
  [o.length, o[0], o["1"], o.nm].join(","),
  ["0" in o, "length" in o, "5" in o, Reflect.has(o, "1")].join(","),
  keys.join(","),
  String(Reflect.get(o, "length")),
  [({ __proto__: [9] }).length, ({ __proto__: [9] })[0], "0" in { __proto__: [9] }].join(","),
].join("|");

// expect: ok
// #596: objects with many keys switch to an owned, in-place-grown shape.
// Exercise every property operation across the switch-over point.
const o: any = {};
const N = 600;
for (let i = 0; i < N; i++) o["k" + i] = i;
const sym = Symbol("s");
o[sym] = "symval";
Object.defineProperty(o, "hidden", { value: 1, enumerable: false, writable: false, configurable: true });
Object.defineProperty(o, "acc", { get() { return 7; }, enumerable: true, configurable: true });
o[5] = "five";
o[1] = "one";

function check(cond: boolean, msg: string) { if (!cond) throw new Error(msg); }

check(Object.keys(o).length === N + 3, "keys length " + Object.keys(o).length);
check(Object.keys(o)[0] === "1" && Object.keys(o)[1] === "5" && Object.keys(o)[2] === "k0", "key order");
check(o.k0 === 0 && o.k299 === 299 && o.k599 === 599, "reads");
check(o[sym] === "symval", "symbol");
check(o.hidden === 1, "hidden read");
try { o.hidden = 2; } catch (e) {}
check(o.hidden === 1, "non-writable");
check(o.acc === 7, "getter");
check(o.hasOwnProperty("k300") && !o.hasOwnProperty("k9999"), "hasOwn");
check("k42" in o && !("nope" in o), "in");
o.k10 = "updated";
check(o.k10 === "updated", "overwrite");
check(delete o.k20 && !("k20" in o) && o.k21 === 21, "delete");
o.k20 = "again";
const ks = Object.keys(o);
check(o.k20 === "again" && ks[ks.length - 1] === "k20", "re-add goes last");
const d = Object.getOwnPropertyDescriptor(o, "hidden")!;
check(d.enumerable === false && d.writable === false, "descriptor");

// a second object built the same way must be independent
const p: any = {};
for (let i = 0; i < N; i++) p["k" + i] = -i;
p.k1 = "p1";
check(o.k2 === 2 && p.k2 === -2 && p.k1 === "p1", "independence of two wide objects");

// inline-cached access to a growing wide object
function getK(x: any) { return x.k3; }
for (let i = 0; i < 5; i++) getK(o);
o.brandNew = 1;
check(getK(o) === 3, "cache after add");
delete o.k3;
check(getK(o) === undefined, "cache after delete");

// freeze a wide object
const f: any = {};
for (let i = 0; i < 300; i++) f["a" + i] = i;
Object.freeze(f);
try { f.a1 = 99; } catch (e) {}
try { f.newKey = 1; } catch (e) {}
check(f.a1 === 1 && f.newKey === undefined && Object.isFrozen(f), "freeze");

// prototype chain: wide prototype
const proto: any = {};
for (let i = 0; i < 300; i++) proto["m" + i] = i;
const child = Object.create(proto);
check(child.m250 === 250, "wide proto read");
child.m250 = "own";
check(child.m250 === "own" && proto.m250 === 250, "shadow");
proto.late = "L";
check(child.late === "L", "proto add visible");

// inline cache across a wide *intermediate* prototype that later shadows the key
const top: any = { shared: "top" };
const mid: any = Object.create(top);
for (let i = 0; i < 300; i++) mid["x" + i] = i;
const leaf = Object.create(mid);
function readShared(x: any) { return x.shared; }
for (let i = 0; i < 5; i++) check(readShared(leaf) === "top", "proto IC warm");
mid.shared = "mid";
check(readShared(leaf) === "mid", "proto IC sees shadow added in place on wide mid");
delete mid.shared;
check(readShared(leaf) === "top", "proto IC after shadow removed");

// JSON roundtrip
const j = JSON.parse(JSON.stringify(p));
check(Object.keys(j).length === N && j.k599 === -599, "json");
"ok";

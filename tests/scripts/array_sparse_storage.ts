// expect: len=100001 ; read=["x","y",null,null] ; in=[true,false,true] ; hasOwn=[true,true,false] ; keys=["0","1","5000","100000","foo"] ; ownKeys=["0","1","5000","100000","length","foo"] ; forIn=["0","1","5000","100000","foo"] ; values=[1,2,"x","y","f"] ; entries=[["0",1],["1",2],["5000","x"],["100000","y"],["foo","f"]] ; gopd={"value":"x","writable":true,"enumerable":true,"configurable":true} ; forEach=["0:1","1:2","5000:x","100000:y"] ; map=[100001,"x!","y!",false,4] ; filter=[1,2,"x","y"] ; reduce=12xy ; indexOf=[100000,5000,true,100000,"x"] ; join=1|,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,1 ; spreadLen=100001 ; iter=[100001,"y"] ; truncate=[6000,"x",null,["0","1","5000","foo"]] ; delete=[null,false,100001,["0","1","100000","foo"]] ; fillUp=[3001,"s",2999,3001,"3000"] ; overwrite=[3000,3001] ; push=[3002,"p","s"] ; pop=["s",3000,null,2999] ; shift=[0,3000,"s",null] ; unshift=[3002,"u","s",null] ; splice=[1,3002,"s","a","b"] ; concat=[3002,"s","t"] ; json=7503 ; fromArr=[2001,7,true] ; accessor=["g",5001] ; freeze=[true,"{\"value\":1,\"writable\":false,\"enumerable\":true,\"configurable\":false}"] ; huge=[4294967295,1,["4294967294"]] ; strKey=[5001,["5000","05000"]] ; fill=[3000,false,10] ; copyWithin=[2,3001] ; super=[5001,"z"] ; reflectSet=[5001,1] ; assign=["5000"] ; objSpread=["5000"] ; destructure=[1500,"d"] ; at=["t","t"] ; every=[true,true] ; flat=[1,2] ; with=[9,3001,true] ; toSorted=[1,3,3001]
// skip-typecheck
// paserati#544: an array write far past the dense end goes to a sparse
// element store instead of growing the dense slice with holes, so storage
// and time track the number of elements, not the index value - and every
// read, enumeration and Array.prototype method sees those elements.
const out = [];
const T = (name, f) => { let r; try { r = f(); } catch (e) { r = "!" + e.constructor.name; } out.push(name + "=" + (typeof r === "string" ? r : JSON.stringify(r))); };
const mk = () => { const a = [1, 2]; a[5000] = "x"; a[100000] = "y"; a.foo = "f"; return a; };
T("len", () => mk().length);
T("read", () => { const a = mk(); return [a[5000], a[100000], a[4999], a[200000]]; });
T("in", () => { const a = mk(); return [5000 in a, 4999 in a, "100000" in a]; });
T("hasOwn", () => { const a = mk(); return [a.hasOwnProperty(5000), Object.hasOwn(a, "100000"), a.hasOwnProperty(3)]; });
T("keys", () => Object.keys(mk()));
T("ownKeys", () => Reflect.ownKeys(mk()));
T("forIn", () => { const k = []; for (const q in mk()) k.push(q); return k; });
T("values", () => Object.values(mk()));
T("entries", () => Object.entries(mk()));
T("gopd", () => Object.getOwnPropertyDescriptor(mk(), 5000));
T("forEach", () => { const r = []; mk().forEach((v, i) => r.push(i + ":" + v)); return r; });
T("map", () => { const m = mk().map(v => v + "!"); return [m.length, m[5000], m[100000], 3 in m, Object.keys(m).length]; });
T("filter", () => mk().filter(() => true));
T("reduce", () => mk().reduce((a, v) => a + v, ""));
T("indexOf", () => { const a = mk(); return [a.indexOf("y"), a.lastIndexOf("x"), a.includes(undefined), a.findIndex(v => v === "y"), a.findLast(v => v === "x")]; });
T("join", () => { const a = []; a[3000] = 1; return a.join("").length + "|" + a.join(); });
T("spreadLen", () => [...mk()].length);
T("iter", () => { let c = 0, last; for (const v of mk()) { c++; if (v !== undefined) last = v; } return [c, last]; });
T("truncate", () => { const a = mk(); a.length = 6000; return [a.length, a[5000], a[100000], Object.keys(a)]; });
T("delete", () => { const a = mk(); delete a[5000]; return [a[5000], 5000 in a, a.length, Object.keys(a)]; });
T("fillUp", () => { const a = []; a[3000] = "s"; for (let i = 0; i < 3000; i++) a[i] = i; return [a.length, a[3000], a[2999], Object.keys(a).length, Object.keys(a)[3000]]; });
T("overwrite", () => { const a = []; a[3000] = "s"; for (let i = 0; i <= 3000; i++) a[i] = i; return [a[3000], Object.keys(a).length]; });
T("push", () => { const a = []; a[3000] = "s"; a.push("p"); return [a.length, a[3001], a[3000]]; });
T("pop", () => { const a = []; a[3000] = "s"; return [a.pop(), a.length, a.pop(), a.length]; });
T("shift", () => { const a = [0]; a[3000] = "s"; return [a.shift(), a.length, a[2999], a[3000]]; });
T("unshift", () => { const a = []; a[3000] = "s"; a.unshift("u"); return [a.length, a[0], a[3001], a[3000]]; });
T("splice", () => { const a = [0]; a[3000] = "s"; const r = a.splice(1, 1, "a", "b"); return [r.length, a.length, a[3001], a[1], a[2]]; });
T("concat", () => { const a = []; a[3000] = "s"; const c = a.concat(["t"]); return [c.length, c[3000], c[3001]]; });
T("json", () => { const a = []; a[1500] = 1; return JSON.stringify(a).length; });
T("fromArr", () => { const a = []; a[2000] = 7; const b = Array.from(a); return [b.length, b[2000], 1999 in b]; });
T("accessor", () => { const a = []; Object.defineProperty(a, "5000", { get() { return "g"; }, configurable: true }); return [a[5000], a.length]; });
T("freeze", () => { const a = []; a[5000] = 1; Object.freeze(a); return [Object.isFrozen(a), JSON.stringify(Object.getOwnPropertyDescriptor(a, 5000))]; });
T("huge", () => { const a = []; a[4294967294] = 1; return [a.length, a[4294967294], Object.keys(a)]; });
T("strKey", () => { const a = []; a["5000"] = 1; a["05000"] = 2; return [a.length, Object.keys(a)]; });
T("fill", () => { const a = new Array(3000); a[2999] = 1; return [a.length, 5 in a, a.fill(0, 2990).filter(v => v === 0).length]; });
T("copyWithin", () => { const a = [1]; a[3000] = 2; a.copyWithin(0, 3000); return [a[0], a.length]; });
T("super", () => { class A extends Array {} class B extends A { m() { super[5000] = "z"; return this; } } const b = new B(); b.m(); return [b.length, b[5000]]; });
T("reflectSet", () => { const a = []; Reflect.set(a, "5000", 1); return [a.length, a[5000]]; });
T("assign", () => { const a = []; a[5000] = 1; return Object.keys(Object.assign({}, a)); });
T("objSpread", () => { const a = []; a[5000] = 1; return Object.keys({ ...a }); });
T("destructure", () => { const a = []; a[1500] = "d"; const [x, ...rest] = a; return [rest.length, rest[1499]]; });
T("at", () => { const a = []; a[3000] = "t"; return [a.at(-1), a.at(3000)]; });
T("every", () => { const a = [1]; a[3000] = 2; return [a.every(v => v > 0), a.some(v => v === 2)]; });
T("flat", () => { const a = [[1]]; a[3000] = [2]; return a.flat(); });
T("with", () => { const a = [1]; a[3000] = 2; const w = a.with(3000, 9); return [w[3000], w.length, 5 in w]; });
T("toSorted", () => { const a = [3]; a[3000] = 1; const t = a.toSorted(); return [t[0], t[1], t.length]; });

out.join(" ; ");

// expect: keys=0,1,z,g,b,4294967295,a ; gopn=0,1,length,z,g,b,4294967295,a ; ownKeys=0,1,length,z,g,b,4294967295,a ; forin=0,1,z,g,b,4294967295,a ; spread=0,1,z,g,b,4294967295,a ; assign=0,1,z,g,b,4294967295,a ; entries=[["0",1],["1",2],["z",1],["g","G"],["b",2],["4294967295","big"],["a",3]] ; values=[1,2,1,"G",2,"big",3] ; readd=0,1,g,b,4294967295,a,z ; toAccessor=0,1,z,g,b,4294967295,a ; toData=0,1,z,g,b,4294967295,a ; exec=0,1,2,index,input,groups,extra ; exec2=0,1,index,input,groups,q:7 ; exec3=0,1,index,groups,q ; frozen=0,1,z,g,b,4294967295,a:true ; stable=0,x,y,z,w,v
// skip-typecheck
// paserati#548: an array's named (non-index) own properties - data or
// accessor - enumerate in creation order everywhere, and `delete arr.x`
// actually deletes (it used to answer false and keep x).
const out = [];
const mk = () => { const a = [1, 2]; a.z = 1; Object.defineProperty(a, "g", { get() { return "G"; }, enumerable: true, configurable: true }); a.b = 2; a[4294967295] = "big"; a.a = 3; return a; };
const a = mk();
out.push("keys=" + Object.keys(a));
out.push("gopn=" + Object.getOwnPropertyNames(a));
out.push("ownKeys=" + Reflect.ownKeys(a).map(String));
{ const k = []; for (const q in a) k.push(q); out.push("forin=" + k); }
out.push("spread=" + Object.keys({ ...a }));
out.push("assign=" + Object.keys(Object.assign({}, a)));
out.push("entries=" + JSON.stringify(Object.entries(a)));
out.push("values=" + JSON.stringify(Object.values(a)));
const d = mk(); delete d.z; d.z = "again"; out.push("readd=" + Object.keys(d));
const e = mk(); Object.defineProperty(e, "b", { get() { return 1; }, enumerable: true, configurable: true }); out.push("toAccessor=" + Object.keys(e));
const f = mk(); Object.defineProperty(f, "g", { value: 5, enumerable: true, configurable: true, writable: true }); out.push("toData=" + Object.keys(f));
const m = /(a)(b)?/.exec("xab"); m.extra = 1; out.push("exec=" + Object.keys(m));
const m2 = /(a)/.exec("a"); m2.q = 1; m2.index = 7; out.push("exec2=" + Object.keys(m2) + ":" + m2.index);
const m3 = /(a)/.exec("a"); m3.q = 1; delete m3.input; out.push("exec3=" + Object.keys(m3));
const fr = mk(); Object.freeze(fr); out.push("frozen=" + Object.keys(fr) + ":" + Object.isFrozen(fr));
const seen = new Set(); for (let i = 0; i < 30; i++) { const x = [1]; x.x = 1; x.y = 2; x.z = 3; x.w = 4; x.v = 5; seen.add(Object.keys(x).join()); } out.push("stable=" + [...seen].join("/"));
out.join(" ; ");

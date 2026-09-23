// expect: live:1,1,1,1,1~live2:2,2,2,2~tag:Module,[object Module]~ext:false,false~same:true,true,true~keys:count,default,fixed,inc|c2,count,fixed,inc,inc2,Symbol(Symbol.toStringTag)~proto:null~set:TypeError,TypeError,TypeError,ok,false,false,true~desc:{"value":2,"writable":true,"enumerable":true,"configurable":false},true,true,1
// skip-typecheck
// paserati#527: module namespace objects and named imports are live views of
// the exporting module's bindings. Namespaces used to hold snapshot copies,
// import() built a fresh object every call (so import(x) !== import(x) and
// neither equalled `import * as`), and that object had no @@toStringTag and
// stayed extensible. Named imports and `export * from` re-exports were
// snapshots too.
import * as st from "./module_live_bindings/live.ts";
import { count, inc } from "./module_live_bindings/live.ts";
import * as rx from "./module_live_bindings/reex.ts";
const dyn = await import("./module_live_bindings/live.ts");
dyn.inc();
const out = [];
out.push("live:" + [st.count, dyn.count, count, rx.c2, rx.count].join(","));
inc();
out.push("live2:" + [st.count, dyn.count, count, rx.c2].join(","));
out.push("tag:" + dyn[Symbol.toStringTag] + "," + Object.prototype.toString.call(st));
out.push("ext:" + Object.isExtensible(dyn) + "," + Object.isExtensible(st));
const again = await import("./module_live_bindings/live.ts");
out.push("same:" + [st === dyn, dyn === again, (await import("./module_live_bindings/reex.ts")) === rx].join(","));
out.push("keys:" + Object.keys(st).join(",") + "|" + Reflect.ownKeys(rx).map(String).join(","));
out.push("proto:" + Object.getPrototypeOf(st));
const t = (f) => { try { f(); return "ok"; } catch (e) { return e.constructor.name; } };
out.push("set:" + [t(() => { st.count = 5; }), t(() => { st.nope = 1; }), t(() => { delete st.count; }), t(() => { delete st.nope; }), Reflect.set(st, "count", 1), Reflect.defineProperty(st, "count", { value: 9 }), Reflect.defineProperty(st, "count", { value: st.count })].join(","));
const d = Object.getOwnPropertyDescriptor(st, "count");
out.push("desc:" + JSON.stringify(d) + "," + ("count" in st) + "," + Object.hasOwn(st, "fixed") + "," + st.default.d);
out.join("~");

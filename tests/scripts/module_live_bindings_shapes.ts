// expect: 1,h0,v0,f0,c0,d,true~11,h1,v1,f1,c1,11,h1,v1,f1,c1~nsOf:true,false,true~empty:true,1,0,Module~cyc:b-updated,a-init,object~strict:TypeError,TypeError,TypeError,true~json:{"count":0,"default":{"d":1},"fixed":"f"},["count=number","default=object","fixed=string","inc=function"]
// skip-typecheck
// paserati#527: live bindings across export shapes - renamed, var, function
// and class exports reassigned by the module, default exports, `export * as`,
// circular imports - plus namespace identity for a module with no exports.
import * as b from "./module_live_bindings/b.ts";
import def, { x, renamed, v, f, C, bump } from "./module_live_bindings/b.ts";
import * as live from "./module_live_bindings/live.ts";
import * as ca from "./module_live_bindings/cyc_a.ts";
import * as cb from "./module_live_bindings/cyc_b.ts";
const out = [];
out.push([x, renamed, v, f(), C.tag, def(), b.default === def].join(","));
bump();
out.push([x, renamed, v, f(), C.tag, b.x, b.renamed, b.v, b.f(), b.C.tag].join(","));
out.push("nsOf:" + (b.nsOfLive === live) + "," + Object.isFrozen(live) + "," + Object.isSealed(live));
const e1 = await import("./module_live_bindings/empty.ts"), e2 = await import("./module_live_bindings/empty.ts");
out.push("empty:" + (e1 === e2) + "," + globalThis.__emptyRuns + "," + Object.keys(e1).length + "," + e1[Symbol.toStringTag]);
out.push("cyc:" + [ca.readB(), cb.readA(), ca.seenFromA].join(","));
const t = (fn) => { try { fn(); return "ok"; } catch (e) { return e.constructor.name; } };
out.push("strict:" + [t(() => { "use strict"; live.count = 1; }), t(() => { Object.defineProperty(live, "count", { value: 1 }); }), t(() => { Object.setPrototypeOf(live, {}); }), Reflect.setPrototypeOf(live, null)].join(","));
out.push("json:" + JSON.stringify(live) + "," + JSON.stringify(Object.entries(live).map(([k, v]) => k + "=" + typeof v)));
out.join("~");

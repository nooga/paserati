// expect: idxSet/sloppy = 1,9~idxSet/strict = 1,9~in/sloppy = true,true,false,true,true,true,true,true~in/strict = true,true,false,true,true,true,true,true~hasOwn/sloppy = true,true,false,true,true,true,true~hasOwn/strict = true,true,false,true,true,true,true~gopd/sloppy = v=1WEC,v=1WEC,v=1WC,acc,v=fnWC~gopd/strict = v=1WEC,v=1WEC,v=1WC,acc,v=fnWC~keys/sloppy = 0,1,x,y|0,1,length,callee,x,y|2|0,1,length,callee,x,y,Symbol(Symbol.iterator),Symbol(s)~keys/strict = 0,1,x,y|0,1,length,callee,x,y|2|0,1,length,callee,x,y,Symbol(Symbol.iterator),Symbol(s)~defProp/sloppy = 5,7,0,y,v=5E~defProp/strict = 5,7,0,y,v=5E~forIn/sloppy = 0,1,x~forIn/strict = 0,1,x~spread/sloppy = {"0":1,"x":1},2~spread/strict = {"0":1,"x":1},2~assignSrc/sloppy = {"0":1,"x":1}~assignSrc/strict = {"0":1,"x":1}~reflect/sloppy = true,3,true,true,8,true,false~reflect/strict = true,3,true,true,8,true,false~del/sloppy = true,,false,true,true,false~del/strict = true,,false,true,true,false~freeze/sloppy = TypeError,1,TypeError,1,true,false,v=1E,v=1E~freeze/strict = TypeError,1,TypeError,1,true,false,v=1E,v=1E~seal/sloppy = !TypeError:Cannot delete property 'x' of #<Object>~seal/strict = !TypeError:Cannot delete property 'x' of #<Object>~noext/sloppy = TypeError,,TypeError,false,false~noext/strict = TypeError,,TypeError,false,false~lengthSet/sloppy = 5,v=5WC,,false~lengthSet/strict = 5,v=5WC,,false~mapped/sloppy = 1,5,0,x~mapped/strict = 1,5,0,x~json/sloppy = {"0":1,"1":"b","x":1}~json/strict = {"0":1,"1":"b","x":1}~entries/sloppy = [["0",1],["x","q"]][1,"q"]~entries/strict = [["0",1],["x","q"]][1,"q"]
// skip-typecheck
// paserati#535: the same matrix run from strict code, where rejected writes
// (frozen, non-extensible) and failed deletes throw.
"use strict";
const sloppy = function (a, b) { return arguments; };
const strict = function (a, b) { "use strict"; return arguments; };
const sym = Symbol("s");
const d = (o, k) => { const x = Object.getOwnPropertyDescriptor(o, k); if (!x) return "none"; return ("value" in x ? "v=" + (typeof x.value === "function" ? "fn" : String(x.value)) : "acc") + (x.writable ? "W" : "") + (x.enumerable ? "E" : "") + (x.configurable ? "C" : ""); };
const tests = {
  idxSet: (mk) => { const a = mk(1, 2); a["x"] = 1; a["0"] = 9; return a["x"] + "," + a[0]; },
  in: (mk) => { const a = mk(1); a.x = 1; a[sym] = 1; return ["x" in a, 0 in a, 1 in a, "length" in a, "callee" in a, Symbol.iterator in a, sym in a, "toString" in a].join(); },
  hasOwn: (mk) => { const a = mk(1); a.x = 1; return [Object.hasOwn(a, "x"), Object.hasOwn(a, "0"), Object.hasOwn(a, "1"), Object.hasOwn(a, "length"), Object.hasOwn(a, "callee"), Object.hasOwn(a, Symbol.iterator), a.hasOwnProperty("x")].join(); },
  gopd: (mk) => { const a = mk(1); a.x = 1; return [d(a, "x"), d(a, "0"), d(a, "length"), d(a, "callee"), d(a, Symbol.iterator)].join(); },
  keys: (mk) => { const a = mk(1, 2); a.x = 1; a[sym] = 2; a.y = 3; return Object.keys(a).join() + "|" + Object.getOwnPropertyNames(a).join() + "|" + Object.getOwnPropertySymbols(a).length + "|" + Reflect.ownKeys(a).map(String).join(); },
  defProp: (mk) => { const a = mk(1); Object.defineProperty(a, "y", { value: 5, enumerable: true }); Object.defineProperty(a, "z", { get() { return 7; }, configurable: true }); return a.y + "," + a.z + "," + Object.keys(a).join() + "," + d(a, "y"); },
  forIn: (mk) => { const a = mk(1, 2); a.x = 1; const k = []; for (const q in a) k.push(q); return k.join(); },
  spread: (mk) => { const a = mk(1); a.x = 1; a[sym] = 2; const o = { ...a }; return JSON.stringify(o) + "," + o[sym]; },
  assignSrc: (mk) => { const a = mk(1); a.x = 1; return JSON.stringify(Object.assign({}, a)); },
  reflect: (mk) => { const a = mk(1); return [Reflect.set(a, "z", 3), Reflect.get(a, "z"), Reflect.has(a, "z"), Reflect.set(a, "0", 8), a[0], Reflect.deleteProperty(a, "z"), "z" in a].join(); },
  del: (mk) => { const a = mk(1); a.x = 1; return [delete a.x, a.x, "x" in a, delete a.nope, delete a[0], 0 in a].join(); },
  freeze: (mk) => { const a = mk(1); a.x = 1; Object.freeze(a); let r; try { a.x = 2; r = "no-throw"; } catch (e) { r = e.constructor.name; } let r2; try { a[0] = 5; r2 = "no-throw"; } catch (e) { r2 = e.constructor.name; } return [r, a.x, r2, a[0], Object.isFrozen(a), Object.isExtensible(a), d(a, "x"), d(a, "0")].join(); },
  seal: (mk) => { const a = mk(1); a.x = 1; Object.seal(a); a.x = 2; return [a.x, Object.isSealed(a), Object.isFrozen(a), delete a.x, d(a, "x")].join(); },
  noext: (mk) => { const a = mk(1); Object.preventExtensions(a); let r; try { a.n = 2; r = "no-throw"; } catch (e) { r = e.constructor.name; } let r2; try { Object.defineProperty(a, "m", { value: 1 }); r2 = "no-throw"; } catch (e) { r2 = e.constructor.name; } return [r, a.n, r2, Object.isExtensible(a), Reflect.set(a, "q", 1)].join(); },
  lengthSet: (mk) => { const a = mk(1, 2); a.length = 5; const r = [a.length, d(a, "length")]; delete a.length; r.push(a.length, "length" in a); return r.join(); },
  mapped: (mk) => { const f = mk === sloppy ? function (p) { arguments.x = 1; p = 5; const r = arguments[0]; Object.defineProperty(arguments, "0", { value: 6 }); return [r, p, Object.keys(arguments).join()].join(); } : function (p) { "use strict"; arguments.x = 1; p = 5; return [arguments[0], p, Object.keys(arguments).join()].join(); }; return f(1); },
  json: (mk) => { const a = mk(1, "b"); a.x = 1; return JSON.stringify(a); },
  entries: (mk) => { const a = mk(1); a.x = "q"; return JSON.stringify(Object.entries(a)) + JSON.stringify(Object.values(a)); },
};
const out = [];
for (const [n, f] of Object.entries(tests)) for (const [mn, mk] of [["sloppy", sloppy], ["strict", strict]]) {
  let r; try { r = String(f(mk)); } catch (e) { r = "!" + e.constructor.name + ":" + e.message; }
  out.push(n + "/" + mn + " = " + r);
}
out.join("~");

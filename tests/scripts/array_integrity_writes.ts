// expect: strictAssign="!TypeError" ; strictAdd="!TypeError" ; sloppy=[[1,2],2] ; noExtAdd="!TypeError" ; noExtExisting=[7] ; nonWritable="!TypeError" ; sealedWrite=[1,5] ; sealedAdd="!TypeError" ; compound="!TypeError" ; compoundSloppy=[1,2] ; destructure="!TypeError" ; forOf="!TypeError" ; reflectSet=[false,false,[1,2]] ; assign="!TypeError" ; push="!TypeError" ; fill="!TypeError" ; lengthStrict="!TypeError" ; nonWritableLen="!TypeError" ; nonWritableLenSloppy=[[5],1] ; sparseFrozen="!TypeError" ; super="!TypeError" ; defineSame=[1,2] ; defineDifferent="!TypeError" ; reflectDefine=false ; sealDelete="!TypeError" ; sealDeleteSloppy=[false,[1,2]] ; sealDesc={"value":1,"writable":true,"enumerable":true,"configurable":false} ; sealRedefine="!TypeError" ; sealToReadonly={"value":1,"writable":false,"enumerable":true,"configurable":false} ; isSealed=[true,false,false,true,true] ; sparseSeal=[false,true] ; reflectDelete=false ; accessorSeal=[true,false,false] ; plain=[3,4,5001]
// skip-typecheck
// paserati#546: writes to an array element respect Object.freeze/seal/
// preventExtensions, non-writable elements and a non-writable length - on
// every path (arr[i] = v, compound/update/destructuring/for-of targets,
// Reflect.set, Object.assign, Array.prototype methods, super[i] = v,
// Object.defineProperty) - silently in sloppy code, with a TypeError in
// strict code. Sealed elements are non-configurable, and isSealed/isFrozen
// test every own property.
const out = [];
const T = (n, f) => { let r; try { r = f(); } catch (e) { r = "!" + e.constructor.name; } out.push(n + "=" + JSON.stringify(r)); };
const fz = () => Object.freeze([1, 2]);
T("strictAssign", () => { "use strict"; const a = fz(); a[0] = 9; return a; });
T("strictAdd", () => { "use strict"; const a = fz(); a[5] = 9; return a; });
T("sloppy", () => { const a = fz(); a[0] = 9; a[5] = 9; return [a, a.length]; });
T("noExtAdd", () => { "use strict"; const b = Object.preventExtensions([1]); b[5] = 1; return b; });
T("noExtExisting", () => { "use strict"; const b = Object.preventExtensions([1]); b[0] = 7; return b; });
T("nonWritable", () => { "use strict"; const c = [1]; Object.defineProperty(c, "0", { writable: false }); c[0] = 5; return c; });
T("sealedWrite", () => { "use strict"; const s = Object.seal([1, 2]); s[1] = 5; return s; });
T("sealedAdd", () => { "use strict"; const s = Object.seal([1]); s[1] = 4; return s; });
T("compound", () => { "use strict"; const a = fz(); a[0] += 5; return a; });
T("compoundSloppy", () => { const a = fz(); a[0] += 5; a[1]++; return a; });
T("destructure", () => { "use strict"; const a = fz(); [a[0]] = [9]; return a; });
T("forOf", () => { "use strict"; const a = fz(); for (a[0] of [7]); return a; });
T("reflectSet", () => { const a = fz(); return [Reflect.set(a, "0", 3), Reflect.set(a, "9", 3), a]; });
T("assign", () => { const a = fz(); Object.assign(a, { 0: 4 }); return a; });
T("push", () => { const a = fz(); a.push(3); return a; });
T("fill", () => { const a = fz(); a.fill(0); return a; });
T("lengthStrict", () => { "use strict"; const a = fz(); a.length = 0; return a; });
T("nonWritableLen", () => { "use strict"; const a = [1]; Object.defineProperty(a, "length", { writable: false }); a[0] = 5; a[1] = 6; return a; });
T("nonWritableLenSloppy", () => { const a = [1]; Object.defineProperty(a, "length", { writable: false }); a[0] = 5; a[1] = 6; return [a, a.length]; });
T("sparseFrozen", () => { "use strict"; const a = []; a[5000] = 1; Object.freeze(a); a[5000] = 2; return a[5000]; });
T("super", () => { "use strict"; class A extends Array { m() { super[0] = 9; return this; } } const a = new A(1, 2); Object.freeze(a); return a.m(); });
T("defineSame", () => { const a = fz(); Object.defineProperty(a, "0", { value: 1 }); return a; });
T("defineDifferent", () => { const a = fz(); Object.defineProperty(a, "0", { value: 3 }); return a; });
T("reflectDefine", () => Reflect.defineProperty(fz(), "0", { value: 3 }));
T("sealDelete", () => { "use strict"; const s = Object.seal([1, 2]); delete s[0]; return s; });
T("sealDeleteSloppy", () => { const s = Object.seal([1, 2]); return [delete s[0], s]; });
T("sealDesc", () => Object.getOwnPropertyDescriptor(Object.seal([1]), 0));
T("sealRedefine", () => { const s = Object.seal([1]); Object.defineProperty(s, "0", { configurable: true }); return s; });
T("sealToReadonly", () => { const s = Object.seal([1]); Object.defineProperty(s, "0", { writable: false }); return Object.getOwnPropertyDescriptor(s, 0); });
T("isSealed", () => [Object.isSealed(Object.seal([1])), Object.isFrozen(Object.seal([1])), Object.isSealed(Object.preventExtensions([1])), Object.isSealed(Object.preventExtensions([])), Object.isFrozen(Object.freeze([1]))]);
T("sparseSeal", () => { const sp = []; sp[5000] = 1; Object.seal(sp); return [delete sp[5000], Object.isSealed(sp)]; });
T("reflectDelete", () => Reflect.deleteProperty(Object.seal([1]), "0"));
T("accessorSeal", () => { const a = [1]; Object.defineProperty(a, "g", { get() { return 1; }, enumerable: true, configurable: true }); Object.seal(a); return [Object.isSealed(a), Object.getOwnPropertyDescriptor(a, "g").configurable, delete a.g]; });
T("plain", () => { const a = [1, 2]; a[0] = 3; a[2] = 4; a[5000] = 1; return [a[0], a[2], a.length]; });
out.join(" ; ");

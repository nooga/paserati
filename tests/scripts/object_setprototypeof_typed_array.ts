// expect: true
// Regression test for #418: Object.setPrototypeOf rejected every
// typed-array-family value (TypedArray/DataView/ArrayBuffer) as
// "non-object", even though typeof/instanceof correctly treated them as
// objects. Fixing that surfaced several sibling bugs in the same code path
// (Array/Map/Set/... prototype mutation silently no-op'd instead of taking
// effect, an explicit null override was indistinguishable from "unset" on
// several read paths, a non-PlainObject prototype override could panic the
// VM, and non-extensible/cyclic-prototype rejection was missing) - this
// file covers all of them, not just the original report.

let customProto: any = { tag: "custom" };

let u8: any = new Uint8Array(3);
Object.setPrototypeOf(u8, customProto);
let test1 = Object.getPrototypeOf(u8) === customProto && u8.tag === "custom";

let i32: any = new Int32Array(3);
Object.setPrototypeOf(i32, customProto);
let test2 = Object.getPrototypeOf(i32) === customProto;

let f64: any = new Float64Array(3);
Object.setPrototypeOf(f64, customProto);
let test3 = Object.getPrototypeOf(f64) === customProto;

let dv: any = new DataView(new ArrayBuffer(8));
Object.setPrototypeOf(dv, customProto);
let test4 = Object.getPrototypeOf(dv) === customProto;

let ab: any = new ArrayBuffer(8);
Object.setPrototypeOf(ab, customProto);
let test5 = Object.getPrototypeOf(ab) === customProto;

// Array prototype assignment must actually take effect, not silently no-op.
let arr: any = [1, 2, 3];
Object.setPrototypeOf(arr, customProto);
let test6 = Object.getPrototypeOf(arr) === customProto;

let m: any = new Map();
Object.setPrototypeOf(m, customProto);
let test7 = Object.getPrototypeOf(m) === customProto;

// An explicit null override must actually cut the chain off - both for
// Object.getPrototypeOf (the InstancePrototypeOverride path) and for real
// method/property lookup (opGetProp's fast paths and handlePrimitiveMethod),
// which is a distinct code path that previously kept resolving to the
// intrinsic prototype's methods even after setPrototypeOf(x, null).
let u8b: any = new Uint8Array(3);
Object.setPrototypeOf(u8b, null);
let test8 =
  Object.getPrototypeOf(u8b) === null && u8b.subarray === undefined;

let arrNull: any = [1];
Object.setPrototypeOf(arrNull, null);
let test9 = Object.getPrototypeOf(arrNull) === null && arrNull.push === undefined;

let mapNull: any = new Map();
Object.setPrototypeOf(mapNull, null);
let test10 = Object.getPrototypeOf(mapNull) === null && mapNull.get === undefined;

let setNull: any = new Set();
Object.setPrototypeOf(setNull, null);
let test11 = Object.getPrototypeOf(setNull) === null && setNull.has === undefined;

let promiseNull: any = Promise.resolve(1);
Object.setPrototypeOf(promiseNull, null);
let test12 =
  Object.getPrototypeOf(promiseNull) === null && promiseNull.then === undefined;

// RegExp's per-instance prototype override was ignored entirely by the
// method-lookup fast path (unlike Array/Map/Set/Promise/TypedArray above,
// which at least respected a TypeObject override before this fix) - it
// always resolved through the intrinsic RegExp.prototype regardless, so
// `.test` kept working even after setPrototypeOf(re, null).
let reNull: any = /a/;
Object.setPrototypeOf(reNull, null);
let test12b = Object.getPrototypeOf(reNull) === null && reNull.test === undefined;

// A non-extensible instance must reject an actual prototype change.
let frozenArr: any = [1];
Object.preventExtensions(frozenArr);
let test13 = false;
try {
  Object.setPrototypeOf(frozenArr, {});
} catch (e: any) {
  test13 = e instanceof TypeError;
}

// ...but a same-value "change" (setting the prototype it already
// effectively has) must still be a no-op success even when non-extensible.
let frozenArr2: any = [1];
let itsProto = Object.getPrototypeOf(frozenArr2);
Object.preventExtensions(frozenArr2);
let test14 = Object.setPrototypeOf(frozenArr2, itsProto) === frozenArr2;

// Direct self-reference must be rejected (distinct from the
// non-extensible case above - both are real, different rejections).
let selfArr: any = [1];
let test15 = false;
try {
  Object.setPrototypeOf(selfArr, selfArr);
} catch (e: any) {
  test15 = e instanceof TypeError;
}

// Setting one instance's prototype to a different exotic-kind instance
// (not a PlainObject) must not crash property lookup afterward.
let crossKind: any = [1];
Object.setPrototypeOf(crossKind, new Map());
let test16 = crossKind.whatever === undefined;

// A getter inherited through Array.prototype must still be reachable
// through ordinary property access (op_getprop.go's migration to the
// shared, accessor-invoking prototype-chain walker).
Object.defineProperty(Array.prototype, "__test418Getter", {
  get: function () {
    return 7;
  },
  configurable: true,
});
let test17 = ([1] as any).__test418Getter === 7;
delete (Array.prototype as any).__test418Getter;

test1 &&
  test2 &&
  test3 &&
  test4 &&
  test5 &&
  test6 &&
  test7 &&
  test8 &&
  test9 &&
  test10 &&
  test11 &&
  test12 &&
  test12b &&
  test13 &&
  test14 &&
  test15 &&
  test16 &&
  test17;

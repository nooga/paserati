// expect: true
// Reflect.has's dispatch across target kinds (pkg/builtins/reflect_has.go)
// used to have no TypeProxy case at all - a Proxy's own 'has' trap never
// ran, so Reflect.has(proxy, key) was unconditionally false regardless of
// what the trap would answer, even though the `in` operator on the same
// Proxy correctly invoked it - and no case (nor a default:) for most
// other object kinds, so Reflect.has on a Map/Set/Arguments/BoundFunction/
// plain Function/NativeFunctionWithProps/Promise target answered false
// unconditionally for any key, own or not.
const checks: boolean[] = [];

// --- Proxy: 'has' trap is invoked, its result is used ---
const trapCalls: string[] = [];
const proxyWithTrap: any = new Proxy([], {
  has(_target: any, key: any) {
    trapCalls.push(String(key));
    return key === "zzz";
  },
});
checks.push("zzz" in proxyWithTrap && Reflect.has(proxyWithTrap, "zzz"));
checks.push(!("yyy" in proxyWithTrap) && !Reflect.has(proxyWithTrap, "yyy"));
checks.push(trapCalls.length === 4); // one call per `in`/Reflect.has above

// --- Proxy: no 'has' trap falls back to the target's own [[HasProperty]] ---
const proxyNoTrap: any = new Proxy({ a: 1 }, {});
checks.push("a" in proxyNoTrap && Reflect.has(proxyNoTrap, "a"));
checks.push(!("b" in proxyNoTrap) && !Reflect.has(proxyNoTrap, "b"));

// --- Proxy: a revoked proxy throws, doesn't silently answer false ---
const revocable = Proxy.revocable({}, {});
revocable.revoke();
let revokedThrew = false;
try {
  Reflect.has(revocable.proxy, "a");
} catch (e: any) {
  revokedThrew = e instanceof TypeError;
}
checks.push(revokedThrew);

// --- Proxy: a trap that throws propagates the real exception ---
const throwingTrapProxy: any = new Proxy({}, {
  has() {
    throw new Error("boom");
  },
});
let trapThrew = false;
try {
  Reflect.has(throwingTrapProxy, "a");
} catch (e: any) {
  trapThrew = e.message === "boom";
}
checks.push(trapThrew);

// --- Proxy: a non-callable trap throws a TypeError ---
const nonCallableTrapProxy: any = new Proxy({}, { has: 42 });
let nonCallableThrew = false;
try {
  Reflect.has(nonCallableTrapProxy, "a");
} catch (e: any) {
  nonCallableThrew = e instanceof TypeError;
}
checks.push(nonCallableThrew);

// --- Proxy: invariant - a false trap answer for a non-configurable own
// target property is rejected with a TypeError, matching `in`. ---
const invariantTarget: any = {};
Object.defineProperty(invariantTarget, "a", {
  value: 1,
  configurable: false,
  enumerable: true,
  writable: true,
});
const invariantProxy: any = new Proxy(invariantTarget, {
  has() {
    return false;
  },
});
let invariantThrew = false;
try {
  Reflect.has(invariantProxy, "a");
} catch (e: any) {
  invariantThrew = e instanceof TypeError;
}
checks.push(invariantThrew);

// --- Map/Set: own "size", a plain assigned property, and inherited
// methods (Map.prototype.get, Set.prototype.add) ---
const m: any = new Map();
m.foo = "bar";
checks.push("size" in m && Reflect.has(m, "size"));
checks.push("get" in m && Reflect.has(m, "get"));
checks.push("foo" in m && Reflect.has(m, "foo"));
checks.push(!("nope" in m) && !Reflect.has(m, "nope"));
const s: any = new Set();
checks.push("add" in s && Reflect.has(s, "add"));

// --- Arguments: length, numeric indices, and an Object.prototype method ---
function withArgs(_a: any, _b: any) {
  checks.push("length" in arguments && Reflect.has(arguments, "length"));
  checks.push("0" in arguments && Reflect.has(arguments, "0"));
  checks.push(!("5" in arguments) && !Reflect.has(arguments, "5"));
  checks.push(
    "toString" in arguments && Reflect.has(arguments, "toString")
  );
}
withArgs(1, 2);

// --- Callables: a plain function/BoundFunction/native constructor all
// find an inherited Function.prototype method, not just their own
// name/length/prototype intrinsics. ---
function plainFn() {}
checks.push("call" in plainFn && Reflect.has(plainFn, "call"));
checks.push("name" in plainFn && Reflect.has(plainFn, "name"));
const bound: any = plainFn.bind(null);
checks.push("call" in bound && Reflect.has(bound, "call"));
checks.push("name" in bound && Reflect.has(bound, "name"));
checks.push("isArray" in Array && Reflect.has(Array, "isArray"));
checks.push("call" in Array && Reflect.has(Array, "call"));

// --- Promise: no own enumerable state, but Promise.prototype.then is found ---
const p: any = Promise.resolve(1);
checks.push("then" in p && Reflect.has(p, "then"));

checks.every((c) => c === true);

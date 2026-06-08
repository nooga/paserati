// no-typecheck
// Subclass-of-native wedge: Array / Map / Set / Iterator instances must carry
// the subclass's prototype, so subclass-added methods are reachable from
// instances and `instanceof Subclass` is true. Pre-fix the implicit derived
// ctor silently exited (OpSpreadNew type-pun panicked on
// TypeNativeFunctionWithProps) and subclass methods were unreachable.
// // no-typecheck because the type-checker still has gaps representing
// subclassed-generic-builtin instance types; the runtime behavior is what
// we're locking in here.

class MyArr extends Array {
    arrMethod() { return "arr-ok"; }
}
let a = new MyArr();
a.push(1, 2);
let arrOk = a.length === 2 && a[0] === 1
    && a.arrMethod() === "arr-ok"
    && (a instanceof MyArr) && (a instanceof Array);

class MyMap extends Map {
    mapMethod() { return "map-ok"; }
}
let m = new MyMap();
m.set("k", 42);
let mapOk = m.size === 1 && m.get("k") === 42
    && m.mapMethod() === "map-ok"
    && (m instanceof MyMap) && (m instanceof Map);

class MySet extends Set {
    setMethod() { return "set-ok"; }
}
let s = new MySet();
s.add("x").add("y");
let setOk = s.size === 2 && s.has("x")
    && s.setMethod() === "set-ok"
    && (s instanceof MySet) && (s instanceof Set);

arrOk && mapOk && setOk;

// expect: true

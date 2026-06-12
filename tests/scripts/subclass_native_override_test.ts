// no-typecheck
// Subclass-of-native: a subclass that OVERRIDES a builtin method name
// (push/set/add) must invoke the override, not the intrinsic. Regression:
// handlePrimitiveMethod resolved Array/Map/Set methods against the realm
// intrinsic prototype before consulting the instance's per-instance prototype,
// so the override was shadowed. Non-overridden builtins must still resolve via
// the chain, and normal (non-subclass) collections must be unaffected.

class S extends Array {
    push(x) { return 123; }
}
let s = new S();
let arrOk = s.push(9) === 123          // override wins over Array.prototype.push
    && typeof s.map === "function";    // inherited builtin still reachable

class MM extends Map {
    set(k, v) { return "mapped"; }
}
let m = new MM();
let mapOk = m.set(1, 2) === "mapped"
    && typeof m.has === "function";

class SS extends Set {
    add(x) { return "added"; }
}
let st = new SS();
let setOk = st.add(1) === "added"
    && typeof st.has === "function";

let plainOk = [1, 2, 3].push(4) === 4; // normal array: push returns new length

arrOk && mapOk && setOk && plainOk;

// expect: true

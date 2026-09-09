// expect: true
// The remaining `handler.AsPlainObject().GetOwn(<trap>)` sites turned up by
// auditing pkg/vm and pkg/builtins for the same pattern #342 fixed for
// OpIn's `has` lookup - the `with`-statement sites are covered separately
// in tests/scripts/proxy_with_trap_lookup_fixes.ts. Each of these had the
// same two bugs:
//
//   1. Own-only trap lookup, not GetMethod-inherited (`.GetOwn` instead of
//      `.Get`/proxyGetTrap) - an inherited trap on the handler's own
//      [[Prototype]] chain was invisible.
//   2. Unguarded AsPlainObject() panics on a TypeDictObject handler (a TS
//      enum or module namespace value at runtime).
//
// Fixed via proxyGetTrap (pkg/vm/vm.go), reused at every site below:
//   - opGetProp (pkg/vm/op_getprop.go): plain property read, `obj.prop`.
//   - OpGetIndex (pkg/vm/vm.go): computed property read, `obj[key]`.
//   - OpDeleteProp (pkg/vm/vm.go): `delete obj.prop`.
//   - vm.Call's TypeProxy case (pkg/vm/vm_init.go) and
//     prepareCallWithGeneratorMode's TypeProxy case (pkg/vm/call.go): the
//     `apply` trap for, respectively, a native call path (Reflect.apply)
//     and a direct call expression (`proxy(...)`).
//
// call.go's site additionally used to have a THIRD divergent behavior:
// a `default: throw "Proxy handler is not an object"` for any handler
// kind besides TypeObject/TypeDictObject (e.g. a Map or Array - both
// legal per the Proxy constructor's own validation, which only requires
// IsObject()). vm.Call's sibling site never threw for this and instead
// silently treated it as "no apply trap" (delegate to target) - the two
// entry points for calling the very same proxy disagreed. Fixed by
// dropping call.go's throw in favor of matching vm.Call (check 6).
//
// Also fixed in this same commit (not a trap-lookup bug, but touched
// while reviewing the same file, pkg/vm/vm.go's proxyHasPropertyFallback):
// the TypeArray case used `index < arrayObj.Length()` instead of
// ArrayHasOwnIndex to decide whether a numeric index is an own property -
// wrong per paserati#176/#178, since `.length` can be inflated by an
// unrelated defineProperty at a distant index without this index itself
// being set. Now matches OpIn's own TypeArray case: checks
// ArrayHasOwnIndex first, and falls through to the prototype chain
// (Array.prototype) on a miss instead of answering outright (checks 7-8).

const checks: boolean[] = [];

// --- 1. opGetProp: inherited `get` trap. ---
{
  const base = {
    get(_t: any, k: string) {
      return k === "a" ? 42 : undefined;
    },
  };
  const p: any = new Proxy({}, Object.create(base));
  checks.push(p.a === 42);
}

// --- 2. OpGetIndex: inherited `get` trap. ---
{
  const base = {
    get(_t: any, k: string) {
      return k === "b" ? 43 : undefined;
    },
  };
  const p: any = new Proxy({}, Object.create(base));
  const key = "b";
  checks.push(p[key] === 43);
}

// --- 3. OpDeleteProp: inherited `deleteProperty` trap. ---
{
  const base = {
    deleteProperty(_t: any, k: string) {
      return k === "c";
    },
  };
  const p: any = new Proxy({ c: 1 }, Object.create(base));
  checks.push(delete p.c);
}

// --- 4. A TypeDictObject handler must not panic for opGetProp/OpGetIndex/
// OpDeleteProp - no trap is found (an enum has no such members), so each
// falls through to the target. ---
{
  enum DictHandler4 {
    X,
    Y,
  }
  const p: any = new Proxy({ v: 9 }, DictHandler4 as any);
  checks.push(p.v === 9);
  checks.push(p["v"] === 9);
  checks.push(delete p.v);
}

// --- 5. apply trap, inherited: both call surfaces for the same proxy -
// a direct call expression (call.go) and Reflect.apply (vm.Call, via
// vm_init.go) - must agree. ---
{
  const base = {
    apply(_t: any, _thisArg: any, args: any[]) {
      return args[0] + args[1] + 1000;
    },
  };
  function orig(a: number, b: number) {
    return a + b;
  }
  const p: any = new Proxy(orig, Object.create(base));
  checks.push(p(1, 2) === 1003);
  checks.push(Reflect.apply(p, undefined, [1, 2]) === 1003);
}

// --- 6. A handler that is a legal Proxy handler (IsObject()) but neither
// TypeObject nor TypeDictObject (here: an Array) must not throw for a
// direct call - matches Reflect.apply on the same proxy (no apply trap
// found either way, so both delegate to the target). ---
{
  function orig6(a: number) {
    return a + 1;
  }
  const p: any = new Proxy(orig6, [] as any);
  checks.push(p(4) === 5);
  checks.push(Reflect.apply(p, undefined, [4]) === 5);
}

// --- 6b. A TypeDictObject handler must not panic for a direct call or
// Reflect.apply either (no apply trap -> delegate to target). ---
{
  enum DictHandler6b {
    X,
    Y,
  }
  function orig6b(a: number) {
    return a + 1;
  }
  const p: any = new Proxy(orig6b, DictHandler6b as any);
  checks.push(p(4) === 5);
  checks.push(Reflect.apply(p, undefined, [4]) === 5);
}

// --- 7. proxyHasPropertyFallback's TypeArray case: an in-range index
// that isn't actually own (length inflated by a distant defineProperty)
// must NOT be reported present through a no-trap Proxy. ---
{
  const arr: any = [1, 2, 3];
  Object.defineProperty(arr, "100", { value: "x", configurable: true });
  const p: any = new Proxy(arr, {});
  checks.push(5 in p === false); // in range (length=101) but not own
  checks.push(100 in p === true); // the actually-defined index
}

// --- 8. Same fallback: an inherited index on Array.prototype must still
// be found via the prototype-chain walk on a miss (matches OpIn). ---
{
  (Array.prototype as any)[5] = "proto-value";
  try {
    const arr2: any = [1, 2, 3];
    const p2: any = new Proxy(arr2, {});
    checks.push(5 in p2 === true);
  } finally {
    delete (Array.prototype as any)[5];
  }
}

checks.every((c) => c === true);

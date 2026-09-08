// expect: true
// Three related, pre-existing bugs in OpIn's STRING-key TypeProxy case
// (pkg/vm/vm.go, under `propKey := propVal.ToString()`) and its
// supporting proxyHasPropertyFallback, found while adding the symbol-key
// sibling of this exact case in an earlier PR in this stack (#340) and
// deliberately left unfixed there since fixing them changes established
// `in`/Reflect.has behavior on a path that PR wasn't scoped to touch.
//
//   1. Own-only trap lookup, not GetMethod-inherited. The case did
//      `proxy.handler.AsPlainObject().GetOwn("has")` - own-only, but per
//      ECMA-262 10.5.7 step 4 the trap comes from GetMethod(handler,
//      "has"), which walks the handler's prototype chain. An inherited
//      `has` trap was invisible to `in` even though Reflect.has
//      (proxyReflectHas, pkg/builtins/reflect_has.go) already used
//      .Get, not .GetOwn, and so already found it - see checks 1-2.
//
//   2. Unguarded AsPlainObject() panics on a TypeDictObject handler. The
//      same line assumed the handler is specifically a TypeObject. A
//      handler that happens to be a TypeDictObject (a TS enum or module
//      namespace value at runtime - pkg/vm/object.go's NewDictObject)
//      made Value.AsPlainObject() panic the entire process - not a
//      catchable JS exception - instead of just answering false (an
//      enum has no "has" member) or falling through to the trap-lookup
//      logic correctly. See check 3.
//
//   3. Fallback coverage gap shared by both key paths.
//      proxyHasPropertyFallback (string keys) and its symbol-key sibling
//      proxyHasSymbolPropertyFallback (added in #340) both only covered
//      TypeProxy/TypeObject/TypeDictObject/TypeArray/TypeRegExp/
//      TypeFunction/TypeClosure as fallback target kinds when a Proxy has
//      no `has` trap - unlike OpIn's own fuller direct-target switch,
//      which also handles Map/Set/Promise/BoundFunction/NativeFunction/
//      NativeFunctionWithProps/Arguments. A no-trap Proxy wrapping one of
//      those seven kinds disagreed with Reflect.has (whose
//      proxyReflectHas correctly recurses into the fully general
//      reflectHas for a no-trap fallback, pkg/builtins/reflect_has.go).
//      See checks 4-10, one per kind, both key kinds where relevant.
//
// Fixed via:
//   - proxy.handler.AsPlainObject().GetOwn("has") replaced with
//     proxyGetTrap(proxy.handler, "has") in OpIn's string-key TypeProxy
//     case (and in proxyHasPropertyFallback's own nested TypeProxy case,
//     for a chain of proxies with no trap anywhere) - the same helper
//     #340 already introduced for the symbol-key path, so both key kinds
//     now share one spec-correct trap-lookup implementation instead of
//     two independently-bugged ones.
//   - proxyHasPropertyFallback and proxyHasSymbolPropertyFallback both
//     extended with TypeMap/TypeSet/TypePromise/TypeBoundFunction/
//     TypeNativeFunction/TypeNativeFunctionWithProps/TypeArguments cases,
//     copying the existing own-table-then-prototype-chain logic from
//     OpIn's own direct-target switches (both key kinds, same function)
//     for each of those kinds - not new logic, the same logic already
//     proven correct for a non-Proxy target of each kind.
//
// tests/scripts/in_symbol_proxy.ts's check 11 (from #340) originally
// pinned the Map-target divergence for a SYMBOL key explicitly; it is
// updated in this same commit to assert the fixed (agreeing) behavior,
// same history as in_symbol_boundfn_nativefn.ts's own pinned-then-flipped
// assertion (PR #338).

const checks: boolean[] = [];

// --- 1. Inherited `has` trap (GetMethod semantics) is found for a
// string key - matches Reflect.has, which already worked here. ---
{
  const base1 = { has() { return true; } };
  const p1: any = new Proxy({}, Object.create(base1));
  checks.push("x" in p1);
  checks.push(Reflect.has(p1, "x") === true);
}

// --- 2. An own (non-inherited) trap still works after the fix - the
// common case must not regress while fixing the inherited one. ---
{
  const p2: any = new Proxy({}, { has() { return true; } });
  checks.push("y" in p2);
}

// --- 3. A TypeDictObject handler (a TS enum at runtime) must not panic
// for a string key, same as the symbol-key case already fixed in #340
// (tests/scripts/in_symbol_proxy.ts, check 9). The enum has no "has"
// member, so this exercises the trap-lookup's TypeDictObject branch and
// falls through to the no-trap fallback against the {} target. ---
{
  enum DictHandler3 { A, B }
  const p3: any = new Proxy({}, DictHandler3 as any);
  checks.push(("x" in p3) === false);
}

// --- 4. Map: own custom property (string key) and "size" itself, both
// via the no-trap fallback. ---
{
  const m4: any = new Map();
  Object.defineProperty(m4, "vis", { value: 1, configurable: true });
  const p4: any = new Proxy(m4, {});
  checks.push("vis" in p4);
  checks.push(Reflect.has(p4, "vis") === true);
  checks.push("size" in new Proxy(new Map([[1, 2]]), {}));
}

// --- 4b. Map: the identical own-custom-property check for a Symbol key -
// this is the exact scenario in_symbol_proxy.ts's check 11 was pinned
// against before this fix. ---
{
  const m4b: any = new Map();
  const sym4b = Symbol("m4b");
  Object.defineProperty(m4b, sym4b, { value: 1, configurable: true });
  const p4b: any = new Proxy(m4b, {});
  checks.push(sym4b in p4b);
  checks.push(Reflect.has(p4b, sym4b) === true);
}

// --- 5. Set: own custom property and "size" through the no-trap
// fallback, for both key kinds. ---
{
  const s5: any = new Set();
  Object.defineProperty(s5, "vis", { value: 1, configurable: true });
  const p5: any = new Proxy(s5, {});
  checks.push("vis" in p5, Reflect.has(p5, "vis") === true);
  const sym5 = Symbol("s5");
  Object.defineProperty(s5, sym5, { value: 1, configurable: true });
  const p5b: any = new Proxy(s5, {});
  checks.push(sym5 in p5b, Reflect.has(p5b, sym5) === true);
  checks.push("size" in new Proxy(new Set([1, 2]), {}));
}

// --- 6. Promise: an own property assigned directly onto the instance
// (Promise has no intrinsic own state, but this is still possible), plus
// an inherited method ("then") through the fallback's prototype walk. ---
{
  const pr6: any = Promise.resolve(1);
  pr6.vis = 1;
  const p6: any = new Proxy(pr6, {});
  checks.push("vis" in p6, Reflect.has(p6, "vis") === true);
  const sym6 = Symbol("pr6");
  pr6[sym6] = 1;
  const p6b: any = new Proxy(pr6, {});
  checks.push(sym6 in p6b, Reflect.has(p6b, sym6) === true);
  checks.push("then" in new Proxy(Promise.resolve(1), {}));
}

// --- 7. BoundFunction: an own custom property, for both key kinds. ---
{
  function orig7(a: number, b: number) {}
  const bound7: any = orig7.bind(null, 1);
  bound7.vis = 1;
  const p7: any = new Proxy(bound7, {});
  checks.push("vis" in p7, Reflect.has(p7, "vis") === true);
  const sym7 = Symbol("bf7");
  bound7[sym7] = 1;
  const p7b: any = new Proxy(bound7, {});
  checks.push(sym7 in p7b, Reflect.has(p7b, sym7) === true);
}

// --- 8. A plain native function (Array.prototype.*-style): an own
// custom property via Object.defineProperty, for both key kinds. ---
{
  const nf8: any = Array.prototype.push;
  Object.defineProperty(nf8, "vis8", { value: 1, configurable: true });
  const p8: any = new Proxy(nf8, {});
  checks.push("vis8" in p8, Reflect.has(p8, "vis8") === true);
  const sym8 = Symbol("nf8");
  Object.defineProperty(nf8, sym8, { value: 1, configurable: true });
  const p8b: any = new Proxy(nf8, {});
  checks.push(sym8 in p8b, Reflect.has(p8b, sym8) === true);
}

// --- 9. A native function with props (a global constructor, e.g.
// Boolean): an own custom property, for both key kinds. ---
{
  Object.defineProperty(Boolean, "vis9", { value: 1, configurable: true });
  const p9: any = new Proxy(Boolean, {});
  checks.push("vis9" in p9, Reflect.has(p9, "vis9") === true);
  const sym9 = Symbol("nfwp9");
  Object.defineProperty(Boolean, sym9, { value: 1, configurable: true });
  const p9b: any = new Proxy(Boolean, {});
  checks.push(sym9 in p9b, Reflect.has(p9b, sym9) === true);
}

// --- 10. Arguments: "length", a numeric index, and an inherited method
// (Object.prototype.toString), all through the no-trap fallback. ---
{
  function f10(a: number, b: number) {
    const p10: any = new Proxy(arguments, {});
    checks.push("length" in p10, Reflect.has(p10, "length") === true);
    checks.push(0 in p10, Reflect.has(p10, 0 as any) === true);
    checks.push("toString" in p10);
  }
  f10(1, 2);
}

checks.every((c) => c === true);

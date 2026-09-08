// expect: true
// OpIn's symbol-key switch (pkg/vm/vm.go) had no `case TypeProxy` at all -
// any symbol-keyed `in` check on a Proxy fell to the default case and
// answered false unconditionally, regardless of a `has` trap or what the
// proxy's target actually has - even though the non-symbol switch's own
// TypeProxy case (same function, just below) already handled this
// correctly, and Reflect.has (proxyReflectHas, pkg/builtins/
// reflect_has.go) already agreed with the target's real answer.
//
//   function fn() {}
//   const p = new Proxy(fn, {});
//   Symbol.hasInstance in p;            // paserati (before): false - Node: true
//   Reflect.has(p, Symbol.hasInstance); // paserati (before): true (correct)
//
// Fixed by adding a symbol-key TypeProxy case mirroring the non-symbol one
// exactly: revoked-proxy TypeError, `has` trap invocation with the
// 10.5.7 step 11 invariant checks (non-configurable own property /
// non-extensible target), and - when no trap is present - a fallback to
// target.[[HasProperty]] via a new proxyHasSymbolPropertyFallback,
// mirroring the existing string-key proxyHasPropertyFallback's exact
// type coverage (TypeProxy/TypeObject/TypeDictObject/TypeArray/TypeRegExp/
// TypeFunction/TypeClosure, default false) rather than OpIn's fuller
// direct-target coverage (which also handles Map/Set/Promise/
// BoundFunction/NativeFunction/NativeFunctionWithProps/Arguments) - see
// check 11 below for the resulting, pre-existing, shared gap this
// intentionally leaves in place for both key kinds.
//
// While implementing the trap lookup, found and fixed two spec
// conformance bugs in the process (not present in the task description,
// found by testing against Node rather than only against the sibling
// string-key case being mirrored):
//   - the trap lookup must accept an INHERITED "has" method (GetMethod
//     semantics, ECMA-262 10.5.7 step 4), not just an own one - see check
//     10.
//   - the trap lookup must not assume the handler is specifically a
//     TypeObject; a handler that happens to be a TypeDictObject (a module
//     namespace or TS enum passed as the handler - not reachable through
//     ordinary object-literal syntax, so not itself exercised here) would
//     otherwise panic in Value.AsPlainObject().
// Both of these bugs equally affect the pre-existing STRING-key TypeProxy
// case just below the new one - deliberately left alone here (fixing it
// changes established behavior on a path this task didn't ask about) and
// flagged as a separate follow-up task alongside check 11's fallback-
// coverage gap (one task, three bullets: the string-key case's own-only
// trap lookup, its unguarded AsPlainObject panic on a TypeDictObject
// handler, and the Map/Set/Promise/callable fallback coverage gap shared
// by both key paths).

const checks: boolean[] = [];

// --- 1. No trap, target is a plain function: Symbol.hasInstance found via
// the FunctionPrototype fallback walk (proxyHasSymbolPropertyFallback's
// TypeFunction case, reusing hasFunctionPrototypeSymbolProperty). ---
{
  function fn() {}
  const p: any = new Proxy(fn, {});
  checks.push(Symbol.hasInstance in p);
  checks.push(Reflect.has(p, Symbol.hasInstance));
}

// --- 2. A `has` trap that always returns true is actually invoked. ---
{
  const p2: any = new Proxy({}, { has(_target: any, _key: any) { return true; } });
  checks.push(Symbol("x") in p2);
}

// --- 3. A `has` trap returning false, with the target genuinely lacking
// the property, answers false without throwing (no invariant violated). ---
{
  const p3: any = new Proxy({}, { has() { return false; } });
  checks.push(!(Symbol("y") in p3));
}

// --- 4. No trap: an own symbol property on the target is found directly
// via the fallback's TypeObject case. ---
{
  const sym = Symbol("own");
  const target: any = {};
  Object.defineProperty(target, sym, { value: 1, configurable: true });
  const p4: any = new Proxy(target, {});
  checks.push(sym in p4);
}

// --- 5. A revoked proxy throws a TypeError for a symbol key, same as the
// existing string-key case. ---
{
  const { proxy, revoke } = (Proxy as any).revocable({}, {});
  revoke();
  let threw = false;
  try {
    Symbol.iterator in proxy;
  } catch (e) {
    threw = e instanceof TypeError;
  }
  checks.push(threw);
}

// --- 6. A `has` trap returning falsy for a non-configurable own property
// on the target throws (ECMA-262 10.5.7 step 11 invariant). ---
{
  const sym = Symbol("nc");
  const target: any = {};
  Object.defineProperty(target, sym, { value: 1, configurable: false });
  const p6: any = new Proxy(target, { has() { return false; } });
  let threw = false;
  try {
    sym in p6;
  } catch (e) {
    threw = e instanceof TypeError;
  }
  checks.push(threw);
}

// --- 7. A nested proxy (no trap at either level) still finds a symbol
// property on the innermost real target - the fallback recurses through
// TypeProxy targets, not just non-Proxy ones. ---
{
  const sym = Symbol("nested");
  const inner: any = {};
  Object.defineProperty(inner, sym, { value: 1, configurable: true });
  const middle: any = new Proxy(inner, {});
  const outer: any = new Proxy(middle, {});
  checks.push(sym in outer);
}

// --- 8. Symbol.hasInstance on a class wrapped in a no-trap proxy: confirms
// this fix composes correctly with the FunctionPrototype-symbol-walk fix
// from an earlier PR in this stack. ---
{
  class C {}
  const p8: any = new Proxy(C, {});
  checks.push(Symbol.hasInstance in p8);
}

// --- 9. A TypeDictObject value (a TS enum, at runtime - pkg/vm/object.go's
// NewDictObject) used as a Proxy handler must not panic. Before the
// proxyGetHasTrap fix, `proxy.handler.AsPlainObject()` was called
// unconditionally, and Value.AsPlainObject() panics for any non-TypeObject
// value - confirmed via this exact reproducer, which crashed the whole
// process (not a catchable JS exception) prior to this fix. The enum has
// no "has" member, so the fallback correctly runs against the {} target
// and finds nothing there either - result is `false`, not a crash. ---
{
  enum DictHandler { A, B }
  const p9: any = new Proxy({}, DictHandler as any);
  checks.push((Symbol("dict") in p9) === false);
}

// --- 10. The has trap is found via GetMethod semantics: an INHERITED trap
// counts, not just an own one (ECMA-262 10.5.7 step 4) - matching
// Reflect.has's existing, already-correct behavior here. ---
{
  const base = { has() { return true; } };
  const p10: any = new Proxy({}, Object.create(base));
  checks.push(Symbol("inherited") in p10);
  checks.push(Reflect.has(p10, Symbol("inherited")) === true);
}

// --- 11. KNOWN, PRE-EXISTING, SHARED GAP (both key kinds) - pinned, not
// fixed here: a Proxy with no trap wrapping a Map target disagrees with
// Reflect.has, because proxyHasSymbolPropertyFallback (and its
// pre-existing string-key sibling, proxyHasPropertyFallback) only covers
// TypeObject/TypeArray/TypeRegExp/TypeFunction/TypeClosure as fallback
// target kinds, not OpIn's fuller direct-target switch (which also
// handles Map/Set/Promise/BoundFunction/NativeFunction/
// NativeFunctionWithProps/Arguments). Verified here for Map specifically;
// by inspection (not individually asserted) the same fallback gap applies
// to the other six kinds too, for the identical reason. Node agrees `in`
// and Reflect.has here; paserati does not yet. This assertion pins
// TODAY's divergence explicitly (matching the established pattern for a
// still-open gap - see the history of the now-fixed pinned assertion in
// in_symbol_boundfn_nativefn.ts) so a future fix has to touch this file
// instead of silently leaving stale documentation.
{
  const m: any = new Map();
  const sym = Symbol("m");
  Object.defineProperty(m, sym, { value: 1, configurable: true });
  const p11: any = new Proxy(m, {});
  const inCheck = sym in p11; // paserati: false (gap) - Node: true
  const reflectCheck = Reflect.has(p11, sym); // true on both
  checks.push(inCheck === false && reflectCheck === true);
}

checks.every((c) => c === true);

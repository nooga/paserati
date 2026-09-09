// expect: true
// Three more sites from the same audit as
// tests/scripts/proxy_with_statement_nested_target_has_fallback.ts (see
// its header for the full background), each a different trap:
//
//   1. OpDeleteProp's Proxy case ("No delete trap, fallback to target"):
//      only handled TypeObject/TypeDictObject directly - `delete
//      proxy.prop` where the target is another trap-less Proxy silently
//      did nothing (`success` stayed whatever it defaulted to) instead
//      of resolving through it. Fixed via the new proxyDeleteFallback
//      helper (pkg/vm/vm.go), mirroring proxyHasPropertyFallback's own
//      TypeProxy recursion for the analogous "has" case.
//
//   2. opSetPropSymbol's Proxy case ("No set trap, fallback to target -
//      implement directly to avoid recursion", pkg/vm/op_setprop.go):
//      only handled TypeObject - a nested Proxy target silently did
//      nothing, not even a write. Its STRING-key sibling (opSetProp)
//      already correctly recursed via `if target.Type() == TypeProxy {
//      return vm.opSetProp(ip, &target, propName, valueToSet) }`; this
//      symbol-key twin never got the same fix. Fixed the same way:
//      `if target.Type() == TypeProxy { return vm.opSetPropSymbol(ip,
//      &target, symKey, valueToSet) }`.
//
//   3. isUnscopable's own Proxy branch ("No get trap, fallback to
//      target", backing @@unscopables consultation during a
//      `with`-statement): gated behind `if proxy.target.Type() ==
//      TypeObject`, so a nested Proxy target's own @@unscopables was
//      never even asked about - the `with` binding always won,
//      regardless of what the target declared unscopable. Fixed via
//      getSymbolPropertyWithReceiver (pkg/vm/vm_init.go, the symbol-key
//      twin of getPropertyWithReceiver - NOT GetSymbolPropertyWithGetter,
//      which despite its name only ever handles a TypeObject or
//      TypeRegExp obj directly and does not recurse through a Proxy
//      target at all, an earlier version of this fix wrongly assumed
//      otherwise and had to be corrected after this exact check still
//      failed).
//
// All verified against real Node output.

const checks: boolean[] = [];

// --- 1. delete through a nested trap-less Proxy target. ---
{
  const inner: any = { d: 1 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  const deleted = delete p2.d;
  checks.push(deleted === true && "d" in inner === false);
}

// --- 2. symbol-keyed property set through a nested trap-less Proxy
// target. ---
{
  const sym = Symbol("s");
  const inner: any = {};
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  p2[sym] = 42;
  checks.push(inner[sym] === 42);
}

// --- 3. Symbol.unscopables lookup through a nested trap-less Proxy
// target during a `with`-statement: the with-object must honor the
// REAL target's @@unscopables and skip the property, leaving the local
// variable untouched. ---
{
  const inner: any = { z: 999 };
  Object.defineProperty(inner, Symbol.unscopables, {
    value: { z: true },
    configurable: true,
  });
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  function f() {
    let z = "local";
    with (p2) {
      return z;
    }
  }
  checks.push(f() === "local");
}

checks.every((c) => c === true);

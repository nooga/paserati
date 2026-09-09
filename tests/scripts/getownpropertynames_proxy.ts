// expect: true
// Object.getOwnPropertyNames had no `case vm.TypeProxy:` at all in its
// switch - it fell to `default: return arr, nil` (empty array) for ANY
// Proxy target, regardless of what the proxy's target actually had:
//
//   const target = { a: 1 };
//   Object.getOwnPropertyNames(new Proxy(target, {})); // before: [] - Node: ["a"]
//   Reflect.ownKeys(new Proxy(target, {}));             // before: [] - Node: ["a"]
//
// This was more severe than it first looked: Reflect.ownKeys's own
// TypeProxy case already had a comment claiming it "delegate[s] to
// Object.getOwnPropertyNames + getOwnPropertySymbols" as "a
// simplification" (implying it was APPROXIMATELY correct, just missing
// the real ownKeys trap's invariant checking) - but that delegation
// target itself had no Proxy support whatsoever, so the "simplification"
// was a complete no-op: Reflect.ownKeys on literally any Proxy, trap or
// no trap, always returned [] before this fix.
//
// Fixed via proxyOwnPropertyKeys (pkg/builtins/object_init.go), a shared
// ECMA-262 10.5.11 [[OwnPropertyKeys]] implementation that
// Object.getOwnPropertyNames, Object.getOwnPropertySymbols, and
// Reflect.ownKeys all delegate to and then filter differently (per spec,
// they all call the same internal method and filter its result - not
// each invoking a possibly-side-effecting trap separately). Implements:
// revoked check (throws), the "ownKeys" trap if present (its result
// validated for String|Symbol-only elements and no duplicates, per
// CreateListFromArrayLike and step 7 - but NOT the fuller
// extensibility/configurable-key invariant validation ECMA-262 10.5.11
// steps 8-16 require of a well-behaved trap's result against its target -
// deliberately deferred, see the follow-up chip this was flagged with),
// or delegation to the proxy's own target when no trap is present.
//
// Fixing this also required fixing a second, independent, pre-existing
// bug the new Proxy delegation would otherwise have silently inherited:
// Object.getOwnPropertyNames's TypeObject case had dead code - an
// unreachable `else if obj.Type() == vm.TypeDictObject` nested INSIDE the
// `case vm.TypeObject:` body, where obj.Type() is always TypeObject, so
// the DictObject branch could never run. A DictObject backs a TypeScript
// `enum` and a module namespace object - both reachable from ordinary
// user code - so `Object.getOwnPropertyNames(SomeEnum)` was already []
// even with no Proxy involved at all; fixed by splitting it into its own
// `case vm.TypeDictObject:` (see check 6 below).

const checks: boolean[] = [];

// --- 1. No trap, plain object target: delegates through, both
// Object.getOwnPropertyNames and Reflect.ownKeys agree. ---
{
  const target: any = { a: 1, b: 2 };
  const p = new Proxy(target, {});
  const names = Object.getOwnPropertyNames(p);
  const keys = Reflect.ownKeys(p);
  checks.push(
    names.length === 2 &&
      names[0] === "a" &&
      names[1] === "b" &&
      keys.length === 2 &&
      keys[0] === "a" &&
      keys[1] === "b"
  );
}

// --- 2. No trap, array target. ---
{
  const target2: any = [1, 2, 3];
  const p2 = new Proxy(target2, {});
  const names = Object.getOwnPropertyNames(p2);
  checks.push(names.length === 4 && names[0] === "0" && names[1] === "1" && names[2] === "2" && names[3] === "length");
}

// --- 3. No trap, function target - exercises the callable-kind
// delegation chain (task_06547fb2) through a Proxy wrapper too. ---
{
  function fn3(a: number) {
    return a;
  }
  const p3: any = new Proxy(fn3, {});
  const names = Object.getOwnPropertyNames(p3);
  checks.push(names.length === 3 && names[0] === "length" && names[1] === "name" && names[2] === "prototype");
}

// --- 4. No trap, native function target (Array.prototype.slice-style,
// non-constructor - no "prototype"). ---
{
  const p4: any = new Proxy(Array.prototype.slice as any, {});
  const names = Object.getOwnPropertyNames(p4);
  checks.push(names.length === 2 && names[0] === "length" && names[1] === "name");
}

// --- 5. No trap, bound function target. ---
{
  function orig5() {}
  const bound5 = orig5.bind(null);
  const p5: any = new Proxy(bound5, {});
  const names = Object.getOwnPropertyNames(p5);
  checks.push(names.length === 2 && names[0] === "length" && names[1] === "name");
}

// --- 6. The DictObject dead-code fix: a TypeScript `enum` compiles to a
// DictObject (pkg/compiler/compile_enum.go), reachable from ordinary user
// code with no Proxy involved at all - this is the form the bug actually
// shipped as (Object.getOwnPropertyNames(SomeEnum) was already [] on its
// own), checked directly first, then again through a Proxy wrapper to
// confirm the new Proxy delegation doesn't silently reintroduce the same
// [] answer for this one target kind while every other kind above works. ---
{
  enum Color6 {
    Red,
    Green,
  }
  const namesDirect = Object.getOwnPropertyNames(Color6);
  // DictObject's own key ORDER isn't guaranteed/tested here (a
  // pre-existing, separate limitation, not part of this fix's scope) -
  // just that the real keys come back at all instead of [].
  const directOk =
    namesDirect.length === 4 &&
    namesDirect.includes("Red") &&
    namesDirect.includes("Green") &&
    namesDirect.includes("0") &&
    namesDirect.includes("1");

  const p6: any = new Proxy(Color6 as any, {});
  const namesViaProxy = Object.getOwnPropertyNames(p6);
  const viaProxyOk =
    namesViaProxy.length === 4 &&
    namesViaProxy.includes("Red") &&
    namesViaProxy.includes("Green") &&
    namesViaProxy.includes("0") &&
    namesViaProxy.includes("1");

  checks.push(directOk && viaProxyOk);
}

// --- 7. Trap present, well-behaved: returns a completely different key
// list than the target actually has - the trap's raw result comes back
// verbatim (this fix does NOT validate it against the target's own
// configurable/extensible invariants - see the follow-up chip). ---
{
  const target7: any = { a: 1 };
  const p7 = new Proxy(target7, {
    ownKeys() {
      return ["b", "c"];
    },
  });
  const keys = Reflect.ownKeys(p7);
  checks.push(keys.length === 2 && keys[0] === "b" && keys[1] === "c");
}

// --- 8. Trap present, mixed string and symbol keys - both halves of
// CreateListFromArrayLike's String|Symbol element restriction are
// exercised, and Reflect.ownKeys returns the trap's result unfiltered. ---
{
  const sym8 = Symbol("s8");
  const target8: any = {};
  const p8: any = new Proxy(target8, {
    ownKeys() {
      return ["x", sym8];
    },
  });
  const keys = Reflect.ownKeys(p8);
  checks.push(keys.length === 2 && keys[0] === "x" && keys[1] === sym8);
}

// --- 9. Trap returns an element that is neither a string nor a symbol:
// CreateListFromArrayLike's element-type restriction throws a TypeError. ---
{
  const p9: any = new Proxy(
    {},
    {
      ownKeys() {
        return ["a", 42];
      },
    }
  );
  let threw = false;
  let isTypeError = false;
  try {
    Reflect.ownKeys(p9);
  } catch (e) {
    threw = true;
    isTypeError = e instanceof TypeError;
  }
  checks.push(threw && isTypeError);
}

// --- 10. Trap returns duplicate keys: ECMA-262 10.5.11 step 7 throws a
// TypeError unconditionally, regardless of the target's own state. ---
{
  const p10: any = new Proxy(
    {},
    {
      ownKeys() {
        return ["a", "a"];
      },
    }
  );
  let threw = false;
  let isTypeError = false;
  try {
    Reflect.ownKeys(p10);
  } catch (e) {
    threw = true;
    isTypeError = e instanceof TypeError;
  }
  checks.push(threw && isTypeError);
}

// --- 10a. Same as check 10, but with a RUNTIME-BUILT duplicate string
// (`k + ""`) rather than two identical literals - dedup keyed on the raw
// value instead of string CONTENT would miss this, since two
// independently-constructed string values holding the same text aren't
// guaranteed to compare equal directly (verified: this exact repro
// initially let the duplicate through unnoticed until dedup was keyed on
// `.ToString()` for strings / the underlying symbol pointer for symbols
// instead). ---
{
  const k10a = "a" + "";
  const p10a: any = new Proxy(
    {},
    {
      ownKeys() {
        return [k10a, "a"];
      },
    }
  );
  let threw = false;
  let isTypeError = false;
  try {
    Reflect.ownKeys(p10a);
  } catch (e) {
    threw = true;
    isTypeError = e instanceof TypeError;
  }
  checks.push(threw && isTypeError);
}

// --- 11. A revoked Proxy throws a TypeError, trap or no trap. ---
{
  const { proxy, revoke } = (Proxy as any).revocable({}, {});
  revoke();
  let threw = false;
  let isTypeError = false;
  try {
    Reflect.ownKeys(proxy);
  } catch (e) {
    threw = true;
    isTypeError = e instanceof TypeError;
  }
  checks.push(threw && isTypeError);
}

// --- 12. Object.getOwnPropertySymbols on a no-trap Proxy over a target
// with a real symbol-keyed own property - the OTHER new TypeProxy case
// (objectGetOwnPropertySymbolsWithVM), distinct target from check 8's
// per this session's "distinct target per assertion" convention. ---
{
  const sym12 = Symbol("s12");
  const target12: any = {};
  target12[sym12] = 1;
  const p12: any = new Proxy(target12, {});
  const syms = Object.getOwnPropertySymbols(p12);
  checks.push(syms.length === 1 && syms[0] === sym12);
}

checks.every((c) => c === true);

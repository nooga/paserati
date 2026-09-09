// expect: true
// OpObjectSpread's Proxy handling (pkg/vm/vm.go, `case OpObjectSpread:`,
// backing `{...proxy}`) checks for an `ownKeys` trap via proxyGetTrap and,
// if found and callable, spreads using it - but when the handler has NO
// `ownKeys` trap at all (a plain `{}` handler, or a TypeDictObject handler
// like a TS enum with no `ownKeys` member), the whole Proxy-handling block
// used to just `continue` past the spread entirely instead of falling back
// to the target's own [[OwnPropertyKeys]] (per ECMA-262 10.5.11 step 5):
//
//   const target = { a: 1, b: 2 };
//   const p = new Proxy(target, {}); // no ownKeys trap
//   const spread = { ...p };
//   spread; // was {} - now correctly { a: 1, b: 2 }, matching Node
//
// Fixed by proxyOwnKeysFallback (pkg/vm/vm.go): when the top-level
// handler has no ownKeys trap, recurse into proxy.Target()'s own keys -
// checking a further-nested Proxy target's OWN ownKeys trap first, not
// just falling through further - the same recursion
// pkg/builtins/json_init.go's getProxyOwnKeys and object_init.go's
// proxyOwnPropertyKeys already do for their own no-ownKeys-trap case
// (pkg/vm can't import pkg/builtins to reuse either directly, so this is
// their pkg/vm-local twin). The resulting key list still goes through the
// EXISTING per-key getOwnPropertyDescriptor/get trap consultation against
// the ORIGINAL top-level proxy's handler - confirmed against Node that
// GetMethod([[OwnPropertyKeys]]) and GetMethod([[Get]]) are independent:
// a Proxy with a `get` trap but no `ownKeys` trap still has that `get`
// trap called during a spread (check 3), same for
// `getOwnPropertyDescriptor` (check 4).
//
// Not fixed here (two separate, pre-existing bugs found while writing
// these checks, confirmed independent - both reproduce with an explicit
// ownKeys trap too, bypassing this fix entirely - and filed as
// follow-ups):
//   - The per-key loop's OWN "no get trap, fall back to target" branch
//     only handles a TypeObject/TypeDictObject target directly, not a
//     further-nested Proxy target - `new Proxy(new Proxy(x, {}), {
//     ownKeys(){...} })` silently reads `undefined` for every key. Check
//     1 below sidesteps this by giving the OUTER proxy an explicit `get`
//     trap (which forwards via Reflect.get, itself already
//     nested-Proxy-aware per PR #341), so the value fetch never falls
//     into that fallback branch - isolating the ownKeys recursion fix
//     this file is actually testing.
//   - proxyOwnKeysFallback's real-object case uses PlainObject.OwnKeys()
//     (enumerable keys only, matching its two pkg/builtins siblings'
//     identical choice), not a "get every own key, filter by
//     descriptor.enumerable afterward" pass - so a non-enumerable own
//     property on a real target is excluded before the
//     getOwnPropertyDescriptor trap even sees it, rather than being
//     excluded BY that trap's answer. Same end result as Node for a real
//     object target (check 4's "hidden" key), reached via a different
//     mechanism - not asserting the trap is asked about "hidden", only
//     that the answer excludes it.

const checks: boolean[] = [];

// --- 1. Basic repro: a plain, trap-less handler must spread the
// target's own enumerable properties instead of {}. ---
{
  const target: any = { a: 1, b: 2 };
  const p: any = new Proxy(target, {});
  const spread: any = { ...p };
  checks.push(spread.a === 1 && spread.b === 2);
}

// --- 2. Nested Proxy target, no ownKeys trap anywhere: key-list
// resolution recurses through the inner Proxy correctly (see header for
// why the outer proxy needs its own `get` trap here). ---
{
  const inner: any = { c: 3 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {
    get(t: any, k: any, r: any) {
      return Reflect.get(t, k, r);
    },
  });
  const spread: any = { ...p2 };
  checks.push(spread.c === 3);
}

// --- 3. get trap still fires per key even though there's no ownKeys
// trap - independent traps per spec. ---
{
  const log: string[] = [];
  const target: any = { a: 1, b: 2 };
  const p: any = new Proxy(target, {
    get(t: any, k: any) {
      log.push(String(k));
      return Reflect.get(t, k);
    },
  });
  const spread: any = { ...p };
  checks.push(spread.a === 1 && spread.b === 2);
  checks.push(log.indexOf("a") !== -1 && log.indexOf("b") !== -1);
}

// --- 4. getOwnPropertyDescriptor trap still fires per (enumerable) key
// even with no ownKeys trap; a non-enumerable own property on the real
// target is excluded either way (see header for the "how"). ---
{
  const log: string[] = [];
  const target: any = { a: 1 };
  Object.defineProperty(target, "hidden", {
    value: 9,
    enumerable: false,
    configurable: true,
  });
  const p: any = new Proxy(target, {
    getOwnPropertyDescriptor(t: any, k: any) {
      log.push(String(k));
      return Reflect.getOwnPropertyDescriptor(t, k);
    },
  });
  const spread: any = { ...p };
  checks.push(spread.a === 1 && spread.hidden === undefined);
  checks.push(log.indexOf("a") !== -1);
}

// --- 5. A TypeDictObject handler (a TS enum at runtime) with no
// ownKeys trap must not panic and must still spread the target. ---
{
  enum DictHandler5 {
    A,
    B,
  }
  const target: any = { x: 5, y: 6 };
  const p: any = new Proxy(target, DictHandler5 as any);
  const spread: any = { ...p };
  checks.push(spread.x === 5 && spread.y === 6);
}

checks.every((c) => c === true);

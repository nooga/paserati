// expect: true
// OpObjectSpread's per-key "no get trap" fallback (pkg/vm/vm.go, the
// Proxy-handling block inside `case OpObjectSpread:`, backing
// `{...proxy}`) fetched each value directly from `proxy.Target()`, but
// only handled a TypeObject/TypeDictObject target:
//
//   target := proxy.Target()
//   if target.Type() == TypeObject {
//       value, _ = target.AsPlainObject().GetOwn(keyStr)
//   } else if target.Type() == TypeDictObject {
//       value, _ = target.AsDictObject().GetOwn(keyStr)
//   } else {
//       value = Undefined
//   }
//
// A target that is itself a Proxy (neither level with a `get` trap)
// silently answered `undefined` for every key instead of recursing into
// that inner proxy's own [[Get]] (trap-or-target, recursively):
//
//   const inner = { c: 3 };
//   const p1 = new Proxy(inner, {});
//   const p2 = new Proxy(p1, { ownKeys() { return ["c"]; } });
//   ({...p2}); // was {} - now { c: 3 }, matching Node
//
// The same shape of bug was independently confirmed via plain property
// access (`p2b.c` where p2b wraps a trap-less Proxy wrapping a plain
// object) in opGetProp's OWN "no get trap" fallback - already fixed by
// routing that one through GetPropertyWithReceiver in the immediately
// preceding PR in this stack (which gave general nested-Proxy-target
// support to that call site as a side effect of a different, unrelated
// fix). This PR's fix does the equivalent for OpObjectSpread's site:
// GetPropertyWithReceiver(proxy.Target(), keyStr, sourceVal) instead of
// the bare GetOwn - receiver stays sourceVal (the ORIGINAL top-level
// proxy the spread started from), matching how the sibling get-trap-call
// branch just above already passes sourceVal as the trap's own receiver
// argument.
//
// checks 1-2: the OpObjectSpread repro above, both with and without an
// ownKeys trap on the OUTER proxy (isolating this fix from the separate
// ownKeys-fallback fix from an earlier PR in this stack).
// check 3: the plain-property-access repro (already fixed, pinned here
// too since it's the same conceptual bug and the task that reported this
// asked for both).
// check 4: three trap-less Proxy levels deep, for both access styles.
// check 5: a `get` trap partway down a longer chain still fires (the
// recursion doesn't just skip past every level to the innermost target).

const checks: boolean[] = [];

// --- 1. Nested trap-less Proxy target, outer proxy HAS an ownKeys
// trap. ---
{
  const inner: any = { c: 3 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {
    ownKeys() {
      return ["c"];
    },
  });
  const spread: any = { ...p2 };
  checks.push(spread.c === 3);
}

// --- 2. Nested trap-less Proxy target, outer proxy has NO ownKeys trap
// either (both fixes composing together). ---
{
  const inner: any = { d: 4 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  const spread: any = { ...p2 };
  checks.push(spread.d === 4);
}

// --- 3. Plain property access (dot notation) through a nested
// trap-less Proxy chain. ---
{
  const p2b: any = new Proxy(new Proxy({ c: 3 }, {}), {});
  checks.push(p2b.c === 3);
}

// --- 4. Three levels deep, still trap-less throughout, both spread and
// dot-notation access. ---
{
  const p3: any = new Proxy(new Proxy(new Proxy({ e: 5 }, {}), {}), {});
  checks.push(p3.e === 5);
  const spread: any = { ...p3 };
  checks.push(spread.e === 5);
}

// --- 5. A `get` trap partway down a longer chain still fires - the
// recursion checks each level's own trap, it doesn't just delegate all
// the way to the innermost real target. ---
{
  const log: string[] = [];
  const innermost: any = { f: 6 };
  const mid: any = new Proxy(innermost, {
    get(t: any, k: any) {
      log.push(String(k));
      return Reflect.get(t, k);
    },
  });
  const outer: any = new Proxy(mid, {});
  checks.push(outer.f === 6);
  checks.push(log.indexOf("f") !== -1);
}

checks.every((c) => c === true);

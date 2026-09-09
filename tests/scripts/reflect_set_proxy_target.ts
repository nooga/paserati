// expect: true
// Reflect.set's "set" closure (pkg/builtins/reflect_init.go) had no
// handling at all for `target` being a Proxy: `target.Type() == TypeProxy`
// passes the gate (Value.IsObject() is a contiguous [TypeObject, TypeProxy]
// range check, so it's true for a Proxy), but neither the old isDataProp
// computation nor the final target-kind switch had a case for TypeProxy -
// so it fell through to an unconditional `return false`, silently doing
// nothing for ANY Proxy target, with or without a `set` trap:
//
//   const target = {};
//   const p = new Proxy(target, {});
//   Reflect.set(p, "x", 5); // before: false, target.x stayed undefined - Node: true, target.x === 5
//
// Fixed via reflectProxySet (mirrors op_setprop.go's opSetProp TypeProxy
// handling - the bytecode `obj.x = v` path, which already got this right
// for that narrower case - but forwards Reflect.set's own `receiver`
// argument to the trap, not necessarily the proxy itself).
//
// Fixing this also required completing the receiver-side half for a
// receiver that itself turns out to be a Proxy - the single most common
// call, Reflect.set(someProxy, key, value) with receiver OMITTED, defaults
// receiver to the proxy itself, so even the "target is a plain object,
// nothing exotic going on" recursive case now routes through a Proxy
// receiver. reflectProxyDefineDataProperty implements that: the receiver
// Proxy's own defineProperty trap if present, else delegate straight to
// CreateDataProperty on ITS OWN target. This superseded (not patched) an
// old inline "receiver is a distinct Proxy" special case that only handled
// a receiver-Proxy WITH a defineProperty trap and otherwise silently fell
// through to the exact task_cd1507d7 bug this file's sibling test already
// covers for other receiver kinds - removed rather than left to keep
// intercepting upstream of the real fix.
//
// Found while fixing this, NOT fixed here (flagged as a follow-up chip):
// Reflect.set's propKey := args[1].ToString() stringifies a Symbol key
// before it ever reaches a Proxy's `set` trap, so
// Reflect.set(proxy, Symbol.iterator, v) hands the trap the string
// "Symbol(Symbol.iterator)" instead of the real Symbol - pre-existing,
// unrelated to target-being-a-Proxy, but this fix is the first place that
// mangled key becomes externally observable to user code (a trap
// function) rather than just internally wrong.
//
// Every check here was verified against real Node.js output.

const checks: boolean[] = [];

// --- 1. Proxy WITH a set trap: the trap receives (target, key, value,
// receiver) - `receiver` is the ORIGINAL Reflect.set receiver argument,
// not the proxy itself - and a well-behaved trap's own write is observed. ---
{
  const target: any = {};
  const receiver: any = { tag: "custom-receiver" };
  let seenTargetMatches = false;
  let seenKey: any = null;
  let seenValue: any = null;
  let seenReceiverMatches = false;
  const p: any = new Proxy(target, {
    set(t: any, k: any, v: any, r: any) {
      seenTargetMatches = t === target;
      seenKey = k;
      seenValue = v;
      seenReceiverMatches = r === receiver;
      Reflect.set(t, k, v);
      return true;
    },
  });
  const ok = Reflect.set(p, "x", 5, receiver);
  checks.push(
    ok === true &&
      seenTargetMatches &&
      seenKey === "x" &&
      seenValue === 5 &&
      seenReceiverMatches &&
      target.x === 5
  );
}

// --- 2. Proxy WITHOUT a set trap, distinct plain receiver: delegates to
// the target, and the receiver-bug fix (task_cd1507d7) still applies -
// the write lands on the receiver, not target. ---
{
  const target2: any = {};
  const p2: any = new Proxy(target2, {});
  const receiver2: any = {};
  const ok = Reflect.set(p2, "y", 7, receiver2);
  checks.push(ok === true && receiver2.y === 7 && !("y" in target2));
}

// --- 3. Proxy WITHOUT a set trap, receiver OMITTED (defaults to the proxy
// itself) - the single most common way to call Reflect.set on a Proxy at
// all. Since a Proxy has no data storage of its own, CreateDataProperty on
// the proxy-as-receiver recurses into its own (trap-less) defineProperty
// delegation, landing on the underlying target. ---
{
  const target3: any = {};
  const p3: any = new Proxy(target3, {});
  const ok = Reflect.set(p3, "z", 9);
  checks.push(ok === true && target3.z === 9 && p3.z === 9);
}

// --- 4. A revoked proxy throws a TypeError. ---
{
  const { proxy, revoke } = (Proxy as any).revocable({}, {});
  revoke();
  let threw = false;
  let isTypeError = false;
  try {
    Reflect.set(proxy, "x", 1);
  } catch (e) {
    threw = true;
    isTypeError = e instanceof TypeError;
  }
  checks.push(threw && isTypeError);
}

// --- 5. A trap returning truish for a target property that is
// non-configurable, non-writable, and already a DIFFERENT value is an
// ECMA-262 10.5.9 invariant violation - throws a TypeError rather than
// silently succeeding. ---
{
  const target4: any = {};
  Object.defineProperty(target4, "x", { value: 1, writable: false, configurable: false });
  const p4: any = new Proxy(target4, {
    set() {
      return true;
    },
  });
  let threw = false;
  let isTypeError = false;
  try {
    Reflect.set(p4, "x", 2);
  } catch (e) {
    threw = true;
    isTypeError = e instanceof TypeError;
  }
  checks.push(threw && isTypeError && target4.x === 1);
}

// --- 6. A trap returning falsish means the set failed - false, no throw,
// and no write happens. ---
{
  const target5: any = {};
  const p5: any = new Proxy(target5, {
    set() {
      return false;
    },
  });
  const ok = Reflect.set(p5, "x", 1);
  checks.push(ok === false && target5.x === undefined);
}

// --- 7. receiver is a DISTINCT Proxy with a defineProperty trap: both the
// getOwnPropertyDescriptor trap (side-effect parity) and the
// defineProperty trap fire, with the exact CreateDataProperty-shaped
// descriptor ({value, writable: true, enumerable: true, configurable:
// true}). Distinct target6/receiverProxy6 from checks 1-6 per this
// session's "distinct target per assertion" convention. ---
{
  const target6: any = {};
  let getOwnPropDescCalled = false;
  let definePropCalled = false;
  let definePropKey: any = null;
  let definePropValue: any = null;
  let definePropWritable: any = null;
  let definePropEnumerable: any = null;
  let definePropConfigurable: any = null;
  const receiverProxy6: any = new Proxy(
    {},
    {
      getOwnPropertyDescriptor() {
        getOwnPropDescCalled = true;
        return undefined;
      },
      defineProperty(t: any, k: any, desc: any) {
        definePropCalled = true;
        definePropKey = k;
        definePropValue = desc.value;
        definePropWritable = desc.writable;
        definePropEnumerable = desc.enumerable;
        definePropConfigurable = desc.configurable;
        Object.defineProperty(t, k, desc);
        return true;
      },
    }
  );
  const ok = Reflect.set(target6, "y", 5, receiverProxy6);
  checks.push(
    ok === true &&
      getOwnPropDescCalled &&
      definePropCalled &&
      definePropKey === "y" &&
      definePropValue === 5 &&
      definePropWritable === true &&
      definePropEnumerable === true &&
      definePropConfigurable === true &&
      receiverProxy6.y === 5
  );
}

// --- 8. receiver is a DISTINCT Proxy with only a getOwnPropertyDescriptor
// trap (no defineProperty trap): falls back to CreateDataProperty on the
// receiver-proxy's OWN target directly. ---
{
  const target7: any = {};
  let called = false;
  const receiverProxy7: any = new Proxy(
    {},
    {
      getOwnPropertyDescriptor() {
        called = true;
        return undefined;
      },
    }
  );
  const ok = Reflect.set(target7, "z", 9, receiverProxy7);
  checks.push(ok === true && called && receiverProxy7.z === 9);
}

// --- 9. A revoked receiver-Proxy (target is a plain object; the
// RECEIVER is the revoked one) also throws a TypeError - not the same
// code path as check 4's revoked target, but the same outcome (verified
// against Node). ---
{
  const { proxy, revoke } = (Proxy as any).revocable({}, {});
  revoke();
  let threw = false;
  let isTypeError = false;
  try {
    Reflect.set({}, "y", 5, proxy);
  } catch (e) {
    threw = true;
    isTypeError = e instanceof TypeError;
  }
  checks.push(threw && isTypeError);
}

checks.every((c) => c === true);

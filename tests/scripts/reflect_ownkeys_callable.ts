// expect: true
// Reflect.ownKeys had two independent problems, found while verifying
// paserati PR #343:
//
// 1. Its object-gate threw on any callable target. Unlike every other
//    Reflect method in this file (get/set/has/apply/construct/...), which
//    all correctly gate on `!target.IsObject() && !target.IsCallable()`,
//    ownKeys gated on `!target.IsObject()` alone - so it threw a TypeError
//    outright for a plain function, a native function, a bound function,
//    or a class, even though every other Reflect method here already
//    accepts them:
//
//      Reflect.ownKeys(Array.prototype.slice); // before: threw - Node: ["length", "name"]
//
// 2. Even past that gate, the switch had no case at all for any callable
//    kind (TypeFunction/TypeClosure/TypeNativeFunction/
//    TypeNativeFunctionWithProps/TypeBoundFunction) - so fixing (1) alone
//    would just fall through and answer [] instead of throwing.
//
// Rather than hand-rolling a third independent per-kind name/length/
// prototype synthesis switch (this codebase's dominant recurring bug
// class this whole multi-PR effort has been chasing down - N independent
// copies of the same per-kind dispatch slowly drifting apart), these five
// kinds now delegate straight to Object.getOwnPropertyNames +
// Object.getOwnPropertySymbols (concatenated - which is exactly the
// ECMA-262 10.1.11 OrdinaryOwnPropertyKeys order Reflect.ownKeys itself
// requires: integer indices, then string keys, then symbol keys, all in
// creation order). TypeObject/TypeDictObject/TypeArray/TypeProxy keep
// their own pre-existing, already-correct logic untouched.
//
// Object.getOwnPropertyNames itself had no case at all for
// TypeNativeFunction (a plain, non-Props native function like
// Array.prototype.slice) or TypeBoundFunction either - the same "kind
// missing from a switch" gap, one level down, fixed in the same commit
// since Reflect.ownKeys's delegation depends on it for exactly those two
// kinds.
//
// Found while verifying, NOT fixed here - a real, narrower, pre-existing
// bug in the untouched TypeFunction/TypeClosure cases, deliberately
// deferred as a follow-up: they synthesize "prototype" unconditionally
// whenever it isn't already a real, explicit own property - even for an
// arrow function or a plain (non-generator) async function, neither of
// which has a "prototype" own property at all per spec (verified against
// Node). Not touched here since it's unrelated to the callable-kind gate/
// switch bug this task is about, and none of Node, this task's stated
// scope, or its listed test targets involve arrow/async functions.

const checks: boolean[] = [];

// --- 1. A plain function: length, name, prototype (the gate no longer
// throws, and TypeFunction/TypeClosure already had correct per-kind
// synthesis via Object.getOwnPropertyNames). ---
{
  function fn(a: number, b: number) { return a + b; }
  const keys = Reflect.ownKeys(fn as any);
  checks.push(
    keys.length === 3 &&
      keys[0] === "length" &&
      keys[1] === "name" &&
      keys[2] === "prototype"
  );
}

// --- 2. A class (compiles to TypeClosure, same shape as check 1 through
// a different surface syntax). ---
{
  class C {}
  const keys = Reflect.ownKeys(C as any);
  checks.push(
    keys.length === 3 &&
      keys[0] === "length" &&
      keys[1] === "name" &&
      keys[2] === "prototype"
  );
}

// --- 3. A native function that is NOT a constructor
// (TypeNativeFunction) - no "prototype" (Node agrees: Array.prototype.slice
// has none). This is one of the two kinds that had NO case at all in
// Object.getOwnPropertyNames before this fix - previously silently []. ---
{
  const keys = Reflect.ownKeys(Array.prototype.slice as any);
  checks.push(keys.length === 2 && keys[0] === "length" && keys[1] === "name");
}

// --- 4. A native constructor (TypeNativeFunctionWithProps,
// IsConstructor === true) - HAS "prototype", unlike check 3's non-
// constructor native function. Not asserting the full key list here:
// paserati's own Array bootstrap registers its statics in a different
// order than Node's (isArray/from/of/fromAsync vs Node's isArray/from/
// fromAsync/of) - an engine implementation detail, not a spec
// requirement, and not something this fix controls (it only touches
// synthesis of length/name/prototype, not the order user code registers
// its own statics in). Asserting the length/name/prototype prefix and
// that more keys follow is what's actually spec-fixed and
// engine-independent. ---
{
  const keys = Reflect.ownKeys(Array as any);
  checks.push(
    keys[0] === "length" &&
      keys[1] === "name" &&
      keys[2] === "prototype" &&
      keys.length > 3
  );
}

// --- 5. A bound function (TypeBoundFunction) - the OTHER kind that had
// no case at all in Object.getOwnPropertyNames before this fix. Per PR
// #343, "name"/"length" are REAL own properties set at bind time (not
// synthesized), and a bound function never has its own "prototype"
// regardless of whether its target is a constructor - verified against
// Node for both a bound plain function AND a bound class. ---
{
  function orig() {}
  const bound = orig.bind(null);
  const keys = Reflect.ownKeys(bound as any);
  checks.push(keys.length === 2 && keys[0] === "length" && keys[1] === "name");
}
{
  class D {}
  const boundClass = (D as any).bind(null);
  const keys = Reflect.ownKeys(boundClass);
  checks.push(keys.length === 2 && keys[0] === "length" && keys[1] === "name");
}

// --- 6. The symbol half of the delegation actually fires: a symbol
// property added to a plain function (own local function, not a shared
// intrinsic, so this can't collide with any other test file's own
// mutations of a real global) shows up in Reflect.ownKeys, positioned
// after every string key per ECMA-262 10.1.11 OrdinaryOwnPropertyKeys.
// Not comparing this against Object.getOwnPropertyNames +
// getOwnPropertySymbols the way checks 1-5 implicitly do (via the fixed
// implementation itself calling exactly those two functions) - doing so
// here would just be asserting the delegation agrees with itself. This
// checks an actual value: the symbol is present, and it comes last. ---
{
  function fn6(a: number) { return a; }
  const sym6 = Symbol("custom6");
  Object.defineProperty(fn6, sym6, {
    value: 1,
    enumerable: false,
    configurable: true,
  });
  const keys = Reflect.ownKeys(fn6 as any);
  checks.push(
    keys.length === 4 &&
      keys[0] === "length" &&
      keys[1] === "name" &&
      keys[2] === "prototype" &&
      keys[3] === sym6
  );
}

// --- 7. The gate itself: before this fix, ALL of the calls above would
// have thrown a TypeError instead of returning key lists - explicitly
// confirm a callable target no longer throws, and a genuinely
// non-object, non-callable primitive still correctly does (per spec,
// Reflect.ownKeys's target must be an object). ---
{
  let threw = false;
  try {
    Reflect.ownKeys(42 as any);
  } catch (e) {
    threw = e instanceof TypeError;
  }
  checks.push(threw);
}

checks.every((c) => c === true);

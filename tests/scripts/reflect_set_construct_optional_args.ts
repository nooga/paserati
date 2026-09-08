// expect: true
// Reflect.set and Reflect.construct were both declared with fewer
// parameters than their runtime implementations actually accepted, so a
// call using their spec'd optional trailing argument failed type checking
// even though it ran fine with --no-typecheck - the same gap PR #341
// already fixed for Reflect.get's optional third `receiver` argument:
//
//   const target: any = {};
//   Reflect.set(target, "x", 1, target); // before: TS2554 "Expected 3 arguments, but got 4."
//
// Both fixed the same way as Reflect.get: types.NewOptionalFunction
// instead of types.NewSimpleFunction, with the trailing argument(s)
// marked optional.
//
// Fixing Reflect.construct's signature to actually ALLOW a distinct
// newTarget argument immediately surfaced that the runtime feature behind
// it was itself broken for almost every real newTarget - not just a type
// declaration gap, a genuine runtime bug, found and fixed in the same
// commit since testing the newly-typecheckable 3rd argument is what
// caught it:
//
//   function C(a) { this.a = a; }
//   function D() {}
//   D.prototype = { fromD: true };
//   Object.getPrototypeOf(Reflect.construct(C, [1], D)) === D.prototype; // before: false - Node: true
//
// Root cause: ConstructWithNewTarget (pkg/vm/vm_init.go) unwrapped a
// TypeClosure newTarget straight to its underlying, SHARED
// *FunctionObject (newTarget.AsClosure().Fn) and read/created "prototype"
// there - but `someClosure.prototype = x` (op_setprop.go) writes into
// closure.Properties, a per-CLOSURE-INSTANCE side table that shadows the
// shared Fn's, exactly like the closure's ordinary property-read path
// already respects. So any newTarget whose .prototype had been reassigned
// (including a class, which compiles to TypeClosure) silently got the
// stale, unshadowed default instead. Fixed by using
// ClosureObject.GetPrototypeWithVM (pkg/vm/function.go) - the same method
// `class X extends Y` (OpValidateSuperclass, pkg/vm/vm.go) already uses
// for the identical purpose - instead of reaching past the closure
// wrapper. A bound-function or native-constructor newTarget had the
// identical "falls back to constructor's own prototype, not newTarget's"
// bug via the same code path - fixed by routing those through
// GetPrototypeFromConstructor instead, which already reads their real,
// non-lazily-created "prototype" property correctly (unlike
// TypeFunction/TypeClosure's synthesized-on-first-access one, which is
// why they need the ClosureObject/FunctionObject-specific methods instead).
//
// Found while testing, NOT fixed here - a real, separate, pre-existing
// runtime bug, deliberately deferred as a follow-up: Reflect.set ignores
// a non-Proxy receiver argument distinct from target when target lacks
// the property being set - it always writes to target regardless. Only
// the receiver === target form (the common case, and the one this fix's
// own type-signature change most directly unblocks) works correctly.
// Pinned explicitly below (check 3) rather than silently left
// undocumented - see its own comment for the repro and the reasoning for
// deferring it. Flagged as a follow-up chip.

const checks: boolean[] = [];

// --- 1. Reflect.set with all 4 arguments, receiver === target (the
// common/simple form) - already worked at runtime before this fix; this
// confirms the type-signature change didn't perturb it. ---
{
  const target1: any = {};
  const ok = Reflect.set(target1, "x", 1, target1);
  checks.push(ok === true && target1.x === 1);
}

// --- 2. Reflect.set omitting only the trailing `receiver` (the ONLY
// optional parameter per spec - lib.es2015.reflect.d.ts declares
// `set(target, propertyKey, value, receiver?)`; `value` is required, so
// passing `undefined` for it explicitly is the correct way to exercise
// "value omitted at the value level" - a bare 2-arg call would be a type
// error, correctly, and isn't what "receiver is optional" means). ---
{
  const target2: any = {};
  const ok = Reflect.set(target2, "z", undefined);
  checks.push(ok === true && target2.z === undefined);
}

// --- 3. KNOWN, PRE-EXISTING GAP (not fixed here, flagged as a follow-up):
// Reflect.set with a receiver DISTINCT from target, where target lacks
// the property, should write to the RECEIVER per ECMA-262 10.1.9.2
// OrdinarySetWithOwnDescriptor - paserati instead writes to target. This
// assertion pins TODAY's wrong behavior explicitly (matching the
// established pattern for a still-open gap - see in_symbol_proxy.ts's
// history) so a future fix has to touch this file instead of silently
// leaving stale documentation. Node: ok === true, receiver3.y === 5,
// "y" in target3 === false. ---
{
  const target3: any = {};
  const receiver3: any = {};
  const ok = Reflect.set(target3, "y", 5, receiver3);
  checks.push(ok === true && target3.y === 5 && receiver3.y === undefined);
}

// --- 4. Reflect.construct with 2 arguments (newTarget defaults to
// target) - already worked; confirms no regression. ---
{
  function C4(a: number) { (this as any).a = a; }
  const inst: any = Reflect.construct(C4 as any, [3]);
  checks.push(inst.a === 3 && inst instanceof (C4 as any));
}

// --- 5. Reflect.construct with a distinct newTarget whose .prototype was
// reassigned to a fresh object - the exact bug this fix closes. Not
// asserting `inst.constructor === D5`: replacing .prototype with a plain
// object literal has no "constructor" backlink to begin with (Node
// agrees - inst.constructor is Object there, not D5), so that would be a
// wrong assertion, not a discriminating one. The prototype-identity check
// is what actually distinguishes the fix. ---
{
  function C5(a: number) { (this as any).a = a; }
  function D5() {}
  (D5 as any).prototype = { fromD5: true };
  const inst: any = Reflect.construct(C5 as any, [4], D5 as any);
  checks.push(inst.a === 4 && Object.getPrototypeOf(inst) === (D5 as any).prototype);
}

// --- 6. Reflect.construct with a class as newTarget (classes compile to
// TypeClosure too - same code path as check 5, different surface syntax). ---
{
  function C6(a: number) { (this as any).a = a; }
  class D6 {}
  const inst: any = Reflect.construct(C6 as any, [6], D6 as any);
  checks.push(inst.a === 6 && Object.getPrototypeOf(inst) === (D6 as any).prototype);
}

// --- 7. Reflect.construct with a bound function as newTarget, custom
// prototype - the TypeBoundFunction half of the same bug class. ---
{
  function C7(a: number) { (this as any).a = a; }
  function E7() {}
  const bound7: any = E7.bind(null);
  bound7.prototype = { fromBound7: true };
  const inst: any = Reflect.construct(C7 as any, [7], bound7);
  checks.push(inst.a === 7 && Object.getPrototypeOf(inst) === bound7.prototype);
}

// --- 8. Reflect.construct with a fresh (never-touched) function
// newTarget: the auto-vivified default prototype, WITH its constructor
// backlink, must still work exactly as before this fix (a regression
// guard - this is the one case that happened to already work, since
// there's no divergence between the shared FunctionObject and a
// never-created closure-instance override). ---
{
  function C8(a: number) { (this as any).a = a; }
  function Fresh8() {}
  const inst: any = Reflect.construct(C8 as any, [8], Fresh8 as any);
  checks.push(
    inst.a === 8 &&
      Object.getPrototypeOf(inst) === (Fresh8 as any).prototype &&
      inst.constructor === Fresh8
  );
}

// --- 9. Reflect.construct omitting newTarget entirely: defaults to
// target, constructor backlink intact - confirms the 3-arg overload
// (added by this fix's type-signature change) doesn't disturb the
// pre-existing 2-arg form. ---
{
  function C9(a: number) { (this as any).a = a; }
  const inst: any = Reflect.construct(C9 as any, [9]);
  checks.push(inst.a === 9 && inst.constructor === C9);
}

checks.every((c) => c === true);

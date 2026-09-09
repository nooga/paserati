// expect: true
// The checker's `in`-based control-flow narrowing rejected type-checking a
// symbol-key `in` check immediately followed by a symbol-key index read on
// the same `any`-typed variable, inside one logical expression - even
// though the variable is declared `any` and should permit any index
// operation regardless of narrowing:
//
//   const arr: any = [1, 2];
//   const s = Symbol("x");
//   console.log(true && s in arr && arr[s] === 1);
//   // before: PS2001 "cannot apply index operator to type has-property-[[Symbol]]"
//   // Node:   type-checks fine, prints false (arr[s] is undefined)
//
// Splitting the same checks across separate statements (assigning `s in
// arr` to a variable first, then indexing `arr[s]` later) avoided the
// error - so the bug was specifically about narrowing applied INLINE
// within the same `&&` chain right before a symbol-key index access, not
// about `in`-narrowing or symbol indexing individually.
//
// Root cause: detectTypeGuard's Pattern 4 ("prop" in x, or a symbol-key `in`
// check) produces a TypeGuard whose NarrowedType is a
// *types.PropertyExistenceMarker - a synthetic marker meant only to FILTER
// a union type's members down to the ones that have the property (see
// applyPositiveTypeNarrowing's UnionType branch, which consumes it via
// typeHasProperty). Outside a union, that marker isn't a real type at all.
// But applyPositiveTypeNarrowing's generic non-union fallback branch -
// `guard.NarrowedType != nil && types.IsAssignable(guard.NarrowedType,
// originalType)` - fired for it anyway whenever originalType was `any` (or
// `unknown`, handled by its own even-more-permissive branch just above),
// since "anything is assignable to Any" trivially includes the marker.
// That replaced `arr`'s real `any` type with the bare marker for the rest
// of the narrowed scope (the remainder of the `&&` chain, per
// applyTypeNarrowingFromCondition's left-to-right composition) - so
// `arr[s]` then type-checked against the marker's synthetic
// "has-property-[[Symbol]]" pseudo-type instead of `any`, and failed.
//
// Fixed in applyPositiveTypeNarrowing (pkg/checker/narrowing.go) by
// special-casing a PropertyExistenceMarker guard on a non-union original
// type: there's nothing for the marker to filter, so it leaves the type
// unchanged (matches the "cannot narrow" no-op path) instead of assigning
// the marker itself. Checks below cover both a symbol key and a string
// key (the same marker type backs both), a missing property/symbol
// (`false` case, not just `true`), and confirm the pre-existing use of
// this marker to narrow a real union still works unaffected.
//
// Deliberately NOT touched: paserati's `typeof x === "string"` on an
// `any`-typed x DOES narrow x to `string` (unlike real TypeScript, which
// keeps `any` as `any` through such a check) - a separate, pre-existing,
// much broader behavior this fix does not change. Only
// PropertyExistenceMarker (the `in`-check guard) was special-cased, since
// unlike a real type such as `string`, it is never itself a valid type to
// assign to a variable - it exists purely to filter union members.

type UnionA = { kind: "a"; value: number };
type UnionB = { kind: "b" };
function describeUnion(x: UnionA | UnionB): number {
  if ("value" in x) {
    return x.value;
  }
  return -1;
}

const checks: boolean[] = [];

// --- 1. The exact reported repro: symbol-key `in` immediately followed by
// a symbol-key index read on the same `any` variable, in one `&&` chain. ---
{
  const arr: any = [1, 2];
  const s = Symbol("x");
  checks.push((true && s in arr && arr[s] === 1) === false);
}

// --- 2. Same shape, but the symbol property actually exists - the `in`
// check is true, and `any` still permits the index read to see the real
// value (not just "doesn't crash"). Uses a plain object rather than an
// array: whether an array's OWN symbol-keyed property round-trips through
// `arr[sym]` at all is unrelated, separately-tracked work (this repo's
// array-symbol-property support lives in its own PR stack) - this check
// is only about the checker's narrowing, not that runtime behavior. ---
{
  const o2: any = {};
  const s2 = Symbol("y");
  o2[s2] = 42;
  checks.push(true && s2 in o2 && o2[s2] === 42);
}

// --- 3. String-key `in` on an `any` variable, same inline-chain shape -
// detectTypeGuard's Pattern 4 produces the identical PropertyExistenceMarker
// for a string key, so this hit the same bug. ---
{
  const o: any = { foo: 1 };
  checks.push(true && "foo" in o && o.foo === 1);
  checks.push(true && "foo" in o && o["foo"] === 1);
}

// --- 4. `in` narrowing negated by a following `||` doesn't crash either -
// sanity check that the fix didn't just special-case the exact `&&` shape
// from the repro. ---
{
  const o2: any = { bar: 2 };
  const result = false || ("bar" in o2 && o2.bar === 2);
  checks.push(result === true);
}

// --- 5. Sanity: `in` narrowing on a genuine UNION still works (the marker
// is still meaningful there - this task must not have broken the one case
// where PropertyExistenceMarker is actually consumed as a filter). ---
{
  checks.push(describeUnion({ kind: "a", value: 7 }) === 7);
  checks.push(describeUnion({ kind: "b" }) === -1);
}

// --- 6. The INVERTED (else-branch) counterpart of check 1/2, on a plain
// identifier - applyInvertedTypeNarrowing's non-member-expression path
// only consumes PropertyExistenceMarker inside its own UnionType branch
// (gated on `originalType.(*types.UnionType)`), so a non-union `any`
// already fell through to that function's final "no inverted narrowing
// applied, return nil" path untouched - unlike the positive side, this
// direction never had a generic Any/Unknown fallback that could leak the
// marker in. Verified by inspection AND here: this must keep working
// exactly as it already did, not because this task changed it. ---
{
  const o3: any = { foo: 1 };
  const s3 = Symbol("z");
  let sawUndefined = false;
  if (s3 in o3) {
    checks.push(false); // s3 was never set - must not reach here
  } else {
    sawUndefined = o3[s3] === undefined;
  }
  checks.push(sawUndefined);

  let sawFoo = false;
  if ("bar" in o3) {
    checks.push(false); // "bar" was never set - must not reach here
  } else {
    sawFoo = o3.foo === 1;
  }
  checks.push(sawFoo);
}

// --- 7. A dotted member-expression base ("prop" in holder.inner), also
// `any` - applyPositiveTypeNarrowing's separate member-expression branch
// (isMemberExpression=true) routes a PropertyExistenceMarker through
// filterUnionByProperty instead of the chain this task patched.
// filterUnionByProperty's own non-union fallback already calls
// typeHasProperty(originalType, name), which returns false for Any (Any
// matches none of typeHasProperty's cases) - so `narrowed` comes back
// false and this path already correctly declines to narrow, leaving
// holder.inner as `any`. Verified by inspection AND here, both branches,
// for the same reason as check 6: this task didn't touch it, but it
// needs to stay covered so a future change to filterUnionByProperty
// can't silently reintroduce the same class of bug for this call site. ---
{
  const holder: { inner: any } = { inner: { foo: 1 } };
  checks.push(true && "foo" in holder.inner && holder.inner.foo === 1);

  if ("bar" in holder.inner) {
    checks.push(false); // "bar" was never set - must not reach here
  } else {
    checks.push(holder.inner.foo === 1);
  }
}

checks.every((c) => c === true);

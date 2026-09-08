// expect: true
// Reflect.has and the `in` operator on arrays used to report an index as
// present merely because it was numerically less than the array's
// `.length`, regardless of whether that index was ever actually set.
// `.length` can be inflated by an entirely unrelated Object.defineProperty
// call at a different (possibly huge) index (paserati#176/#178: an index
// past maxDenseArrayDefineIndex, 2^24, is tracked in the array's side
// property/getter/setter maps instead of the dense `elements` slice, but
// still extends `.length` to index+1) - so an array holding a handful of
// real elements plus one property at a huge index made every unset index
// in between falsely report as present.
//
// Fixed via a shared ArrayHasOwnIndex helper (pkg/vm/value.go) that checks
// real presence - a dense value (HasIndex), an own accessor at that index
// (checked regardless of dense/sparse, since DefineAccessorProperty never
// touches `elements` at all), or a sparse own data property
// (GetOwnPropertyDescriptor) - instead of `idx < arr.Length()`, in both
// pkg/vm/vm.go's OpIn and pkg/builtins/reflect_init.go's Reflect.has.
const checks: boolean[] = [];

// Small in-range false positive: index 500 was never set, but the
// unrelated defineProperty at 1000000 inflates .length past it.
const small: any[] = [];
Object.defineProperty(small, "1000000", {
  value: "z",
  writable: true,
  enumerable: true,
  configurable: true,
});
checks.push(!("500" in small));
checks.push(!Reflect.has(small, "500"));
checks.push("1000000" in small);
checks.push(Reflect.has(small, "1000000"));
checks.push("length" in small);
checks.push(Reflect.has(small, "length"));

// Huge sparse index scenario: a handful of real dense elements plus one
// property at a genuinely huge index (past maxDenseArrayDefineIndex).
// Every unset index in between - both within and beyond the dense range -
// must report absent; the dense elements and the sparse index itself must
// still report present.
const huge: any[] = [1, 2, 3];
Object.defineProperty(huge, "4294967294", {
  value: "z",
  writable: true,
  enumerable: true,
  configurable: true,
});
checks.push("0" in huge && Reflect.has(huge, "0"));
checks.push("2" in huge && Reflect.has(huge, "2"));
checks.push(!("3" in huge) && !Reflect.has(huge, "3")); // in-range (< length), never set
checks.push(!("1000000" in huge) && !Reflect.has(huge, "1000000")); // in-range, never set
checks.push("4294967294" in huge && Reflect.has(huge, "4294967294"));
checks.push("length" in huge && Reflect.has(huge, "length"));

// A non-enumerable property is still an OWN property - `in`/Reflect.has
// don't filter by enumerability (unlike Object.keys et al).
const hidden: any[] = [];
Object.defineProperty(hidden, "10", {
  value: "hidden",
  writable: true,
  enumerable: false,
  configurable: true,
});
checks.push("10" in hidden && Reflect.has(hidden, "10"));

// An own accessor (get/set, no `value`) is a real own property too, at
// any index - a plain in-bounds one (DefineAccessorProperty never touches
// `elements`, even in-bounds) and a sparse one past dense storage.
const accessorInBounds: any[] = [1, 2, 3];
Object.defineProperty(accessorInBounds, "1", {
  get() {
    return 42;
  },
  enumerable: true,
  configurable: true,
});
checks.push("1" in accessorInBounds && Reflect.has(accessorInBounds, "1"));

const accessorSparse: any[] = [1, 2, 3];
Object.defineProperty(accessorSparse, "10000", {
  get() {
    return 42;
  },
  enumerable: true,
  configurable: true,
});
checks.push(
  "10000" in accessorSparse && Reflect.has(accessorSparse, "10000")
);
checks.push(
  !("9999" in accessorSparse) && !Reflect.has(accessorSparse, "9999")
);

// A plain dense array with no accessors/sparse entries at all must keep
// behaving exactly as before - the common case, unaffected.
const plain = [1, 2, 3];
checks.push("0" in plain && Reflect.has(plain, "0"));
checks.push("2" in plain && Reflect.has(plain, "2"));
checks.push(!("3" in plain) && !Reflect.has(plain, "3"));
checks.push(!("-1" in plain) && !Reflect.has(plain, "-1"));

// Known, pre-existing, out-of-scope gap (identical before and after this
// fix - confirmed by diffing against an unmodified build): a numeric key
// that isn't an own array index never falls through to walk the
// prototype chain, so an index set directly on Array.prototype is
// invisible to both `in` and Reflect.has here, unlike Node (where it's
// `true`). This fix only tightens the OWN-property check (real presence
// vs. "numerically less than .length") - it does not add the missing
// prototype fallback, which is a separate, broader change (`in` is
// HasProperty, not HasOwnProperty) and is asserted here as the current,
// unchanged behavior rather than left silently uncovered.
(Array.prototype as any)[999] = "inherited";
const withInheritedIndex = [1, 2, 3];
checks.push(!("999" in withInheritedIndex));
checks.push(!Reflect.has(withInheritedIndex, "999"));
delete (Array.prototype as any)[999];

checks.every((c) => c === true);

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
//
// This file also covers two follow-up fixes to the same two functions:
// `in`/Reflect.has now correctly implement HasProperty (own OR inherited -
// a numeric or named key absent as an own property falls through to the
// prototype chain, via the new exported vm.HasPropertyOnPrototypeChain),
// and Reflect.has now checks an array's own named (non-index) properties
// at all (a plain data property, or an own accessor - via the new shared
// ArrayHasOwnNamedProperty, used by both `in` and Reflect.has so they
// can't drift from each other), and handles a symbol-keyed lookup instead
// of stringifying every key and reporting false unconditionally.
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

// Follow-up fix: `in`/Reflect.has on arrays now correctly implement
// HasProperty (own OR inherited), not just HasOwnProperty. A numeric key
// that isn't an own array index falls through to the prototype chain
// (Array.prototype and beyond) instead of reporting absent outright - so
// an index set directly on Array.prototype IS found, matching Node. (This
// used to be a known, pre-existing gap, pinned here as `false` - inverted
// now that it's fixed, rather than deleted, so a regression back to the
// old behavior fails this test instead of going unnoticed.)
(Array.prototype as any)[999] = "inherited";
const withInheritedIndex = [1, 2, 3];
checks.push("999" in withInheritedIndex);
checks.push(Reflect.has(withInheritedIndex, "999"));
delete (Array.prototype as any)[999];

// The single most common instance of the same prototype-fallback gap: an
// inherited METHOD. Reflect.has(arr, "map") was false for every array,
// regardless of the array's own contents, since Reflect.has's array
// branch had no prototype fallback at all (only "length" and a numeric
// index were ever checked).
checks.push("map" in plain && Reflect.has(plain, "map"));
checks.push("join" in plain && Reflect.has(plain, "join"));

// Follow-up fix: Reflect.has never checked an array's own NAMED
// (non-index) property at all - a plain data property...
const withNamed: any = [1, 2, 3];
withNamed.foo = "bar";
checks.push("foo" in withNamed && Reflect.has(withNamed, "foo"));
// ...or an own named accessor (same DefineAccessorProperty-never-touches-
// `properties` reasoning as the numeric-index case above, just for a
// string key instead) - this was ALSO missing from `in` itself (not just
// Reflect.has), caught by making both share one ArrayHasOwnNamedProperty
// helper rather than fixing Reflect.has to merely match whatever `in`
// happened to do.
const withNamedAccessor: any[] = [1, 2, 3];
Object.defineProperty(withNamedAccessor, "foo", {
  get() {
    return 42;
  },
  enumerable: true,
  configurable: true,
});
checks.push(
  "foo" in withNamedAccessor && Reflect.has(withNamedAccessor, "foo")
);

// A key that's genuinely absent everywhere (own, and on the prototype
// chain) must still report false - the prototype-chain fallback must not
// turn into a false positive for everything.
checks.push(!("totallyMadeUp" in plain) && !Reflect.has(plain, "totallyMadeUp"));

// Follow-up fix: Reflect.has(arr, someSymbol) always stringified the key
// and fell through the whole switch with hasProperty left false,
// regardless of whether the symbol was `Symbol.iterator` (found via the
// prototype chain) or an own symbol-keyed property.
checks.push(Symbol.iterator in plain && Reflect.has(plain, Symbol.iterator));
const ownSymbol = Symbol("x");
const withOwnSymbol: any = [1, 2, 3];
withOwnSymbol[ownSymbol] = 42;
checks.push(ownSymbol in withOwnSymbol && Reflect.has(withOwnSymbol, ownSymbol));
const unusedSymbol = Symbol("unused");
checks.push(!(unusedSymbol in plain) && !Reflect.has(plain, unusedSymbol));

// The inflated-.length false positive this file originally covered must
// still be fixed now that the array branch also walks the prototype
// chain - the fallback must trigger only once ArrayHasOwnIndex has
// already said "no", never take priority over it.
checks.push(!("500" in small) && !Reflect.has(small, "500"));

checks.every((c) => c === true);

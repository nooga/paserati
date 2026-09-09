// expect: true
// Two related gaps in how arrays' own properties get enumerated/copied,
// both stemming from the same root causes as this session's earlier
// array-property fixes:
//
// Gap 1 - a named ENUMERABLE ACCESSOR property (installed via
// Object.defineProperty(arr, "foo", {get, enumerable: true})) was
// invisible to every operation that collects an array's named keys by
// walking ArrayObject.NamedPropertyKeys() alone - a plain-data-only
// store (see DefineAccessorProperty's own doc comment: converting a
// name to an accessor deletes its NamedPropertyKeys()/`properties`
// entry entirely, moving it to getters/setters instead, visible only
// via AccessorKeys()). Confirmed missing from Object.keys,
// Object.values, Object.entries, Object.getOwnPropertyNames,
// Reflect.ownKeys, and for-in:
//
//   const arr = [1, 2];
//   Object.defineProperty(arr, "foo", { get() { return 5; }, enumerable: true });
//   Object.keys(arr);
//   // before: ["0", "1"]           - missing "foo"
//   // Node:   ["0", "1", "foo"]
//
// Object.values/Object.entries had a broader version of the same gap:
// they never walked ANY named property at all, accessor or plain, so
// even a plain `arr.foo = "bar"` (no accessor involved) was dropped.
//
// Fixed by adding a shared arrayNamedKeys(a, enumerableOnly) helper
// (pkg/builtins/object_init.go, mirroring the existing arraySparseIndices
// helper's shape/doc-comment) that walks AccessorKeys() then
// NamedPropertyKeys(), and a matching helper in pkg/vm/vm.go's own
// for-in case (package vm has its own, separate hand-rolled duplicate of
// this same array-property-collection logic, same as arraySparseIndices
// does - see that function's own comment in both files).
//
// Gap 2 - Object.assign didn't invoke a numeric-INDEX accessor (one
// installed AT an existing element, e.g.
// Object.defineProperty(arr, "1", {get, enumerable: true})) at all -
// its dense-range copy loop checked HasOwnIndexProperty (which does
// consult accessors) to decide whether the index exists, then read the
// VALUE via arrObj.Get(i), which returns the stale `elements` slot
// directly with no accessor check:
//
//   const arr = [1, 2, 3];
//   Object.defineProperty(arr, "1", { get() { return 99; }, enumerable: true });
//   arr[1];                    // 99 - ordinary index access already gets this right
//   Object.assign({}, arr)[1]; // before: 2 (stale element) - Node: 99
//
// Object.values/Object.entries had the identical dense-range gap.
// Object-literal spread's TypeArray case (this session's earlier fix)
// already got this right from the start - locked in below as a sanity
// check, not a fix.
//
// Fixed with a new arrayDenseIndexValue helper (the dense-range sibling
// of the pre-existing arraySparseIndexValue), used by Object.assign's
// dense-index copy loop and Object.values/entries' own dense-index
// loops.
//
// Object.getOwnPropertyDescriptors' TypeArray case had an even bigger
// version of gap 1 (it collected NO named key at all, plain or
// accessor) plus its own separate bug: it looped `i := 0; i <
// arr.Length()` instead of bounding the per-index scan to
// DenseLength() and walking arraySparseIndices for anything beyond it -
// the exact paserati#176/#178 multi-billion-iteration hang shape every
// other array-key-collecting function in this file already guards
// against, since a sparse index far past DenseLength() extends
// arr.Length() without growing `elements`. Fixed the same way as the
// others: DenseLength() bound + arraySparseIndices(enumerableOnly:
// false) + arrayNamedKeys(enumerableOnly: false).
//
// Every check below was verified against real Node.js output.

const checks: boolean[] = [];

// --- 1. Object.keys omits a named enumerable accessor property. ---
{
  const arr: any = [1, 2];
  Object.defineProperty(arr, "foo", {
    get() {
      return 5;
    },
    enumerable: true,
    configurable: true,
  });
  checks.push(JSON.stringify(Object.keys(arr)) === '["0","1","foo"]');
}

// --- 2. Object.values omits a plain named property entirely (not an
// accessor at all - the broader half of the gap: this loop didn't walk
// ANY named property before the fix, not just accessor ones). Kept as
// its own array (rather than combined with an accessor property) since
// paserati doesn't track true creation order across the separate
// plain-data/accessor stores a named property can live in - Node
// orders "plain" before "acc" here because that's insertion order, a
// property this fix deliberately doesn't take on; see check 3 below for
// a same-array combination that only depends on membership, not order. ---
{
  const arr2: any = [1, 2];
  arr2.plain = "p";
  checks.push(JSON.stringify(Object.values(arr2)) === "[1,2,\"p\"]");
}

// --- 3. Object.entries omits a named accessor property entirely, and
// the getter must actually be invoked (not a stale/absent slot). ---
{
  const arr3: any = [1, 2];
  let calls3 = 0;
  Object.defineProperty(arr3, "acc", {
    get() {
      calls3++;
      return "a";
    },
    enumerable: true,
    configurable: true,
  });
  checks.push(
    JSON.stringify(Object.entries(arr3)) ===
      '[["0",1],["1",2],["acc","a"]]' && calls3 === 1
  );
}

// --- 4. Object.getOwnPropertyNames omits a named enumerable accessor
// property (getOwnPropertyNames wants every own key regardless of
// enumerability, so this must include a NON-enumerable one too). ---
{
  // Two separate arrays, not one array carrying both accessor names:
  // AccessorKeys() (pkg/vm/value.go) walks Go maps (getters/setters),
  // whose iteration order is randomized per run - two accessor names on
  // the same array would make an exact-order assertion here flaky
  // (paserati doesn't track a named property's true creation order
  // across separate stores at all - see check 2's comment - so this
  // isn't a fixable ordering gap, just something a test must avoid
  // depending on when a single array holds more than one same-store
  // named key).
  const arr4a: any = [1, 2];
  Object.defineProperty(arr4a, "acc", {
    get() {
      return "a";
    },
    enumerable: true,
    configurable: true,
  });
  const arr4b: any = [1, 2];
  Object.defineProperty(arr4b, "hiddenAcc", {
    get() {
      return "h";
    },
    enumerable: false,
    configurable: true,
  });
  checks.push(
    JSON.stringify(Object.getOwnPropertyNames(arr4a)) ===
      '["0","1","length","acc"]' &&
      JSON.stringify(Object.getOwnPropertyNames(arr4b)) ===
        '["0","1","length","hiddenAcc"]'
  );
}

// --- 5. Reflect.ownKeys omits a named enumerable accessor property
// (delegates to the same collection logic as getOwnPropertyNames, so
// this locks in that the delegation actually picked up the fix). ---
{
  const arr5: any = [1, 2];
  Object.defineProperty(arr5, "acc", {
    get() {
      return "a";
    },
    enumerable: true,
    configurable: true,
  });
  checks.push(
    JSON.stringify(Reflect.ownKeys(arr5)) === '["0","1","length","acc"]'
  );
}

// --- 6. for-in omits a named enumerable accessor property, and skips a
// named NON-enumerable one (for-in only visits enumerable keys). ---
{
  const arr6: any = [1, 2];
  Object.defineProperty(arr6, "acc", {
    get() {
      return "a";
    },
    enumerable: true,
    configurable: true,
  });
  Object.defineProperty(arr6, "hiddenAcc", {
    get() {
      return "h";
    },
    enumerable: false,
    configurable: true,
  });
  const seen: string[] = [];
  for (const k in arr6) {
    seen.push(k);
  }
  checks.push(JSON.stringify(seen) === '["0","1","acc"]');
}

// --- 7. Object.assign doesn't invoke a numeric-index accessor - copies
// the stale underlying element instead of calling the getter. ---
{
  const arr7: any = [1, 2, 3];
  let calls7 = 0;
  Object.defineProperty(arr7, "1", {
    get() {
      calls7++;
      return 99;
    },
    enumerable: true,
    configurable: true,
  });
  const copy7 = Object.assign({}, arr7);
  checks.push(copy7[1] === 99 && calls7 === 1);
}

// --- 8. Object.values/Object.entries also miss an index accessor's
// real value (same dense-range gap as Object.assign). ---
{
  const arr8: any = [1, 2, 3];
  Object.defineProperty(arr8, "0", {
    get() {
      return "zero";
    },
    enumerable: true,
    configurable: true,
  });
  checks.push(
    JSON.stringify(Object.values(arr8)) === '["zero",2,3]' &&
      JSON.stringify(Object.entries(arr8)) ===
        '[["0","zero"],["1",2],["2",3]]'
  );
}

// --- 9. Sanity: object-literal spread already invoked a dense-index
// accessor correctly before this fix (this session's earlier
// Object.assign/spread PR) - must still work, unchanged. ---
{
  const arr9: any = [1, 2, 3];
  let calls9 = 0;
  Object.defineProperty(arr9, "1", {
    get() {
      calls9++;
      return 99;
    },
    enumerable: true,
    configurable: true,
  });
  const spread9: any = { ...arr9 };
  checks.push(spread9[1] === 99 && calls9 === 1);
}

// --- 10. Sanity: a NON-enumerable named accessor is still excluded from
// Object.keys/values/entries/Object.assign/spread (only
// getOwnPropertyNames/Reflect.ownKeys/getOwnPropertyDescriptors want
// non-enumerable own keys) - the enumerableOnly gate on the new
// arrayNamedKeys helper must still work in both directions. ---
{
  const arr10: any = [1, 2];
  let calls10 = 0;
  Object.defineProperty(arr10, "hidden", {
    get() {
      calls10++;
      return "h";
    },
    enumerable: false,
    configurable: true,
  });
  const keys10 = Object.keys(arr10);
  const values10 = Object.values(arr10);
  const assign10 = Object.assign({}, arr10);
  const spread10: any = { ...arr10 };
  checks.push(
    !keys10.includes("hidden") &&
      values10.length === 2 &&
      !("hidden" in assign10) &&
      !("hidden" in spread10) &&
      calls10 === 0
  );
}

// --- 11. Combined: an array carrying all four property shapes at once
// (dense-index accessor, sparse-index accessor, named accessor, named
// plain) must agree with Node across every operation in one shot - a
// single-shape probe can pass while a combination still double-counts
// or drops a key two different loops both claim (or both skip). ---
{
  const arr11: any = [1, 2, 3];
  Object.defineProperty(arr11, "1", {
    get() {
      return 99;
    },
    enumerable: true,
    configurable: true,
  });
  Object.defineProperty(arr11, "1000", {
    get() {
      return "sparse";
    },
    enumerable: true,
    configurable: true,
  });
  Object.defineProperty(arr11, "namedAcc", {
    get() {
      return "na";
    },
    enumerable: true,
    configurable: true,
  });
  arr11.namedPlain = "np";

  const expectedKeys = '["0","1","2","1000","namedAcc","namedPlain"]';
  const expectedValues = '[1,99,3,"sparse","na","np"]';
  const expectedEntries =
    '[["0",1],["1",99],["2",3],["1000","sparse"],["namedAcc","na"],["namedPlain","np"]]';
  const expectedGopn =
    '["0","1","2","1000","length","namedAcc","namedPlain"]';
  const expectedAssignSpread =
    '{"0":1,"1":99,"2":3,"1000":"sparse","namedAcc":"na","namedPlain":"np"}';

  const forin: string[] = [];
  for (const k in arr11) {
    forin.push(k);
  }

  checks.push(
    JSON.stringify(Object.keys(arr11)) === expectedKeys &&
      JSON.stringify(Object.values(arr11)) === expectedValues &&
      JSON.stringify(Object.entries(arr11)) === expectedEntries &&
      JSON.stringify(Object.getOwnPropertyNames(arr11)) === expectedGopn &&
      JSON.stringify(Reflect.ownKeys(arr11)) === expectedGopn &&
      JSON.stringify(forin) === expectedKeys &&
      JSON.stringify(Object.assign({}, arr11)) === expectedAssignSpread &&
      JSON.stringify({ ...arr11 }) === expectedAssignSpread
  );
}

// --- 12. Object.getOwnPropertyDescriptors omitted every named key
// entirely (plain or accessor) - checked individually (not via a
// whole-object JSON.stringify) so this doesn't depend on the same
// cross-store/map-iteration ordering check 4 had to avoid. ---
{
  const arr12: any = [1, 2];
  arr12.plain = "p";
  Object.defineProperty(arr12, "acc", {
    get() {
      return "a";
    },
    set(_v: any) {},
    enumerable: true,
    configurable: true,
  });
  const descs12 = Object.getOwnPropertyDescriptors(arr12);
  const has0 = "0" in descs12;
  const has1 = "1" in descs12;
  const hasLength = "length" in descs12;
  checks.push(
    descs12.plain.value === "p" &&
      descs12.plain.enumerable === true &&
      descs12.plain.writable === true &&
      typeof descs12.acc.get === "function" &&
      typeof descs12.acc.set === "function" &&
      descs12.acc.value === undefined &&
      descs12.acc.enumerable === true &&
      has0 &&
      has1 &&
      hasLength
  );
}

// --- 13. Object.getOwnPropertyDescriptors must not hang on a huge
// sparse index (the same paserati#176/#178 shape every other
// array-key-collecting operation in this file already guards against -
// this function's own array-key loop used to scan 0..arr.Length()
// directly instead of bounding to DenseLength() + arraySparseIndices).
// No wall-clock assertion here - a hardware-dependent timing check is
// its own kind of flaky. If the scan were still unbounded, this would
// hang and fail the whole suite at the timeout level, which is signal
// enough; this just asserts the result is still correct. ---
{
  const arr13: any = [1, 2];
  Object.defineProperty(arr13, "1000000000", {
    value: "far",
    enumerable: true,
    configurable: true,
    writable: true,
  });
  const descs13 = Object.getOwnPropertyDescriptors(arr13);
  checks.push(
    descs13["1000000000"].value === "far" &&
      descs13.length.value === 1000000001
  );
}

checks.every((c) => c === true);

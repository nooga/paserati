// expect: true
// Object.assign(target, arr) only ever copied an array's indexed elements
// (dense + sparse, paserati#176/#178) plus (for an object/dict target)
// "length" - it never copied a named string property (`arr.foo = ...`,
// plain or an accessor via Object.defineProperty) or ANY symbol-keyed
// property at all (plain or accessor), regardless of enumerability.
// Object-literal spread (`{...arr}`) had an even broader version of the
// same gap: a TypeArray source was excluded entirely, before the
// string-key/symbol-key copying loops even started, so it contributed
// NOTHING - not even the indexed elements:
//
//   const arr = [1, 2, 3]; arr.foo = "bar";
//   const sym = Symbol("s"); arr[sym] = "sym-val";
//
//   Object.assign({}, arr);
//   // before: {0: 1, 1: 2, 2: 3}                      - missing foo AND the symbol
//   // Node:   {0: 1, 1: 2, 2: 3, foo: "bar"} plus the symbol (verified below)
//
//   ({...arr});
//   // before: {}                                       - missing everything, even the indices
//   // Node:   {0: 1, 1: 2, 2: 3, foo: "bar"} plus the symbol
//
// Fixed by adding named-property (accessor-then-plain, mirroring how
// DefineAccessorProperty removes a converted key from the plain-data
// store entirely) and symbol-property (via ArrayObject's own symbol
// storage - ArrayDefineOwnSymbolProperty, pkg/vm/array_props.go) copy
// loops to Object.assign's TypeArray source branch
// (pkg/builtins/object_init.go), and a whole new TypeArray case to
// object-literal spread's bytecode handler (pkg/vm/vm.go) covering the
// same set of properties - the two must stay in sync, since ECMAScript
// defines both operations in terms of the same CopyDataProperties
// abstract operation.
//
// Both fixes call an accessor's getter (never copy the bare data slot
// underneath it - paserati#274's rule, already applied elsewhere) and
// skip a non-enumerable property entirely (own-enumerable-properties is
// CopyDataProperties' own filter, not a data/accessor distinction).
//
// Every check below was verified against real Node.js output.

const checks: boolean[] = [];

// --- 1. A plain enumerable named string property (`arr.foo = ...`) is
// copied by both operations. ---
{
  const arr: any = [1, 2];
  arr.foo = "bar";
  const viaAssign = Object.assign({}, arr);
  const viaSpread: any = { ...arr };
  checks.push(viaAssign.foo === "bar" && viaSpread.foo === "bar");
}

// --- 2. A non-enumerable named string property (defined via
// Object.defineProperty) is NOT copied by either operation. ---
{
  const arr2: any = [1, 2];
  Object.defineProperty(arr2, "hidden", {
    value: "h",
    enumerable: false,
    configurable: true,
  });
  const viaAssign2 = Object.assign({}, arr2);
  const viaSpread2: any = { ...arr2 };
  checks.push(
    !("hidden" in viaAssign2) &&
      !("hidden" in viaSpread2) &&
      viaAssign2.hidden === undefined &&
      viaSpread2.hidden === undefined
  );
}

// --- 3. A named ENUMERABLE ACCESSOR property invokes its getter exactly
// once per operation - the value copied is what the getter returns, not
// a stale/absent data slot (accessors are stored separately from plain
// data - see DefineAccessorProperty's own doc comment). ---
{
  const arr3: any = [1, 2];
  let calls = 0;
  Object.defineProperty(arr3, "computed", {
    get() {
      calls++;
      return 42;
    },
    enumerable: true,
    configurable: true,
  });
  const viaAssign3 = Object.assign({}, arr3);
  const viaSpread3: any = { ...arr3 };
  checks.push(
    viaAssign3.computed === 42 && viaSpread3.computed === 42 && calls === 2
  );
}

// --- 4. A named NON-enumerable accessor property is not copied, and its
// getter is never invoked (no observable side effect from skipping it). ---
{
  const arr4: any = [1, 2];
  let calls4 = 0;
  Object.defineProperty(arr4, "hiddenAcc", {
    get() {
      calls4++;
      return 99;
    },
    enumerable: false,
    configurable: true,
  });
  const viaAssign4 = Object.assign({}, arr4);
  const viaSpread4: any = { ...arr4 };
  checks.push(
    !("hiddenAcc" in viaAssign4) && !("hiddenAcc" in viaSpread4) && calls4 === 0
  );
}

// --- 5. A plain enumerable symbol-keyed property (`arr[sym] = v`) is
// copied by both operations - the exact shape of the originally reported
// bug. ---
{
  const arr5: any = [1, 2, 3];
  const sym5 = Symbol("plain");
  arr5[sym5] = "sym-val";
  const viaAssign5 = Object.assign({}, arr5);
  const viaSpread5: any = { ...arr5 };
  checks.push(viaAssign5[sym5] === "sym-val" && viaSpread5[sym5] === "sym-val");
}

// --- 6. A non-enumerable symbol-keyed data property (defined via
// Object.defineProperty) is NOT copied by either operation. ---
{
  const arr6: any = [1, 2];
  const sym6 = Symbol("hidden");
  Object.defineProperty(arr6, sym6, {
    value: "hv",
    enumerable: false,
    configurable: true,
  });
  const viaAssign6 = Object.assign({}, arr6);
  const viaSpread6: any = { ...arr6 };
  checks.push(
    !(sym6 in viaAssign6) &&
      !(sym6 in viaSpread6) &&
      viaAssign6[sym6] === undefined &&
      viaSpread6[sym6] === undefined
  );
}

// --- 7. A symbol-keyed ENUMERABLE ACCESSOR property (get/set via
// Object.defineProperty) invokes its getter, exactly once per operation. ---
{
  const arr7: any = [1, 2];
  const sym7 = Symbol("symacc");
  let calls7 = 0;
  Object.defineProperty(arr7, sym7, {
    get() {
      calls7++;
      return "sav";
    },
    enumerable: true,
    configurable: true,
  });
  const viaAssign7 = Object.assign({}, arr7);
  const viaSpread7: any = { ...arr7 };
  checks.push(
    viaAssign7[sym7] === "sav" && viaSpread7[sym7] === "sav" && calls7 === 2
  );
}

// --- 8. A symbol-keyed NON-enumerable accessor property is not copied,
// and its getter is never invoked. ---
{
  const arr8: any = [1, 2];
  const sym8 = Symbol("hiddensymacc");
  let calls8 = 0;
  Object.defineProperty(arr8, sym8, {
    get() {
      calls8++;
      return "nope";
    },
    enumerable: false,
    configurable: true,
  });
  const viaAssign8 = Object.assign({}, arr8);
  const viaSpread8: any = { ...arr8 };
  checks.push(!(sym8 in viaAssign8) && !(sym8 in viaSpread8) && calls8 === 0);
}

// --- 9. Sanity: the array's own indexed elements are still copied
// alongside its named/symbol properties (spread's TypeArray case is
// brand new - it must not have dropped the basic indexed-copy behavior
// while adding named/symbol coverage). ---
{
  const arr9: any = [10, 20, 30];
  arr9.tag = "meta";
  const viaAssign9 = Object.assign({}, arr9);
  const viaSpread9: any = { ...arr9 };
  checks.push(
    viaAssign9[0] === 10 &&
      viaAssign9[1] === 20 &&
      viaAssign9[2] === 30 &&
      viaAssign9.tag === "meta" &&
      viaSpread9[0] === 10 &&
      viaSpread9[1] === 20 &&
      viaSpread9[2] === 30 &&
      viaSpread9.tag === "meta"
  );
}

// --- 10. A named string key that LOOKS numeric but is beyond the
// canonical array-index bound (2^32 - 2, ECMA-262 6.1.7's definition of
// an array index) is an ordinary named property, not an index - and must
// land in the named-property copy loop, not the sparse-index loop, which
// rejects it as too big. Both loops filter with the same "is this a
// canonical array index" predicate the sparse-index loop itself uses, so
// a key like this can't fall in the gap between the two and vanish. ---
{
  const arr10: any = [1, 2];
  arr10["4294967295"] = "past-bound";
  const viaAssign10 = Object.assign({}, arr10);
  const viaSpread10: any = { ...arr10 };
  checks.push(
    viaAssign10["4294967295"] === "past-bound" &&
      viaSpread10["4294967295"] === "past-bound"
  );
}

checks.every((c) => c === true);

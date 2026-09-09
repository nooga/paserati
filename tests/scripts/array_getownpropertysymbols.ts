// expect: true
// THE BIGGEST-BLAST-RADIUS FIX HERE: opGetPropSymbol's TypeArray case
// (pkg/vm/op_getprop.go) - the VM READ PATH for `arr[sym]` - used to skip
// straight to the Array.prototype chain without ever consulting the
// array's OWN symbolProps at all, even though opSetPropSymbol's sibling
// TypeArray case (pkg/vm/op_setprop.go) already wrote `arr[sym] = v`
// there correctly:
//
//   const arr = [1, 2, 3]; const sym = Symbol("s"); arr[sym] = 42;
//   arr[sym]; // before: undefined (or a same-named Array.prototype
//             //         value, if one existed) - Node: 42
//
// This was found while verifying that `Reflect.get(arr, sym)` (which
// already worked, via a different Go-side code path) agreed with
// `arr[sym]` (the bytecode path, which didn't) - the exact "N call sites
// for the same operation slowly drift apart" bug class this whole
// session keeps finding. Left unfixed, this would have made
// Object.getOwnPropertySymbols's own new TypeArray case (below) report a
// symbol key that user code could never actually read back through
// ordinary `arr[sym]` syntax - a dishonest half of a fix.
//
// A third, sibling gap found the same way:
// Object.getOwnPropertyDescriptor(arr, sym) (objectGetOwnPropertyDescriptorWithVM,
// pkg/builtins/object_init.go) had no symbol-key branch for TypeArray
// either - it fell through the (propName-based, meaningless for a
// symbol) index/length checks and then a switch with no TypeArray case,
// landing on undefined even though the property demonstrably exists:
//
//   Object.getOwnPropertyDescriptor(arr, sym);
//   // before: undefined
//   // Node:   {value: 42, writable: true, enumerable: true, configurable: true}
//
// Object.getOwnPropertySymbols had no case at all for TypeArray - its
// if/else-if chain covered TypeObject/TypeFunction/TypeClosure/
// TypeNativeFunction/TypeNativeFunctionWithProps/TypeBoundFunction/
// TypeProxy and fell through for everything else, silently returning []:
//
//   const arr = [1, 2, 3];
//   arr[Symbol("s")] = 42;
//   Object.getOwnPropertySymbols(arr).length; // before: 0 - Node: 1
//
// ArrayObject (pkg/vm/value.go) already stored symbol-keyed properties -
// GetSymbolProp/SetSymbolProp/HasOwnSymbolProp already worked, that's how
// `arr[sym] = v` and `sym in arr` worked at all - but had no enumerator
// for them at all. Fixed by adding ArrayObject.OwnSymbolKeys(), backed by
// a new symbolPropOrder []*SymbolObject field that SetSymbolProp appends
// to (only on first insertion) and DeleteSymbolProp splices out of, so
// symbols enumerate in creation order per ECMA-262 10.1.11
// OrdinaryOwnPropertyKeys (verified against Node: insertion order, and
// symbols always sort after every string key regardless of when each was
// added relative to the other).
//
// Reflect.ownKeys's own TypeArray case had a SEPARATE, independent bug:
// it hand-rolled its own index/sparse-index/"length" logic and never
// included named (non-index) string properties at all, on top of having
// no symbol coverage either. Rather than bolting a third independent
// symbol-enumeration call site onto that hand-rolled logic, it was
// rewritten to delegate to Object.getOwnPropertyNamesWithVM +
// Object.getOwnPropertySymbolsWithVM instead - mirroring the delegation
// already used for the five callable kinds (task_06547fb2) - which is a
// verified pure superset of the old logic (identical index/sparse-index/
// length handling) plus both gaps closed at once.
//
// NOTE on named (non-symbol) property ordering: this task deliberately
// does NOT assert an order between multiple named string properties like
// `arr.foo` and `arr.baz` on an array. ArrayObject.NamedPropertyKeys()
// (pkg/vm/value.go) enumerates its `properties` map with a plain,
// untracked `for k := range a.properties`, which is Go-map-iteration-
// order (i.e. genuinely randomized per run) rather than creation order -
// a separate, pre-existing bug already present in
// Object.getOwnPropertyNames before this task touched anything, merely
// newly SURFACED in Reflect.ownKeys by this task's delegation choice
// (Reflect.ownKeys's old TypeArray case never included named properties
// at all, so it couldn't previously expose this). Filed as a follow-up
// chip rather than fixed here, since it's unrelated to symbol handling.
// Checks below only assert symbol-keyed ordering/positioning and named-
// property MEMBERSHIP, never named-property ORDER.

const checks: boolean[] = [];

// --- 1. Single symbol property on a plain array. ---
{
  const arr: any = [1, 2, 3];
  const sym = Symbol("s");
  arr[sym] = 42;
  const syms = Object.getOwnPropertySymbols(arr);
  checks.push(syms.length === 1 && syms[0] === sym && arr[sym] === 42);
}

// --- 2. Multiple symbol properties enumerate in creation order (the
// symbolPropOrder mechanism this fix added - the one check that actually
// discriminates "has an enumerator" from "has an ORDERED enumerator"). ---
{
  const arr2: any = [1, 2, 3];
  const s1 = Symbol("s1");
  const s2 = Symbol("s2");
  arr2[s1] = "a";
  arr2[s2] = "b";
  const syms2 = Object.getOwnPropertySymbols(arr2);
  checks.push(syms2.length === 2 && syms2[0] === s1 && syms2[1] === s2);
}

// --- 3. Reflect.ownKeys on an array with a symbol property: the symbol
// comes after every string key (indices + "length"), per ECMA-262
// 10.1.11 OrdinaryOwnPropertyKeys. ---
{
  const arr3: any = [1, 2, 3];
  const sym3 = Symbol("s3");
  arr3[sym3] = 99;
  const keys3 = Reflect.ownKeys(arr3);
  checks.push(
    keys3.length === 5 &&
      keys3[0] === "0" &&
      keys3[1] === "1" &&
      keys3[2] === "2" &&
      keys3[3] === "length" &&
      keys3[4] === sym3
  );
}

// --- 4. Named string property + symbol together: membership only for
// the named key (order between multiple named keys is a separate,
// pre-existing, deliberately-not-fixed-here bug - see header comment),
// but the symbol's POSITION (always last, after every string key) is
// asserted since that ordering guarantee doesn't depend on the broken
// map iteration at all. ---
{
  const arr4: any = [1, 2];
  arr4.foo = "bar";
  const sym4 = Symbol("s4");
  arr4[sym4] = "c";
  const keys4 = Reflect.ownKeys(arr4);
  const stringKeys = keys4.slice(0, keys4.length - 1);
  checks.push(
    keys4.length === 5 &&
      keys4[keys4.length - 1] === sym4 &&
      stringKeys.indexOf("0") !== -1 &&
      stringKeys.indexOf("1") !== -1 &&
      stringKeys.indexOf("length") !== -1 &&
      stringKeys.indexOf("foo") !== -1
  );
}

// --- 5. Proxy wrapping an array with a symbol property, no trap: the
// no-trap delegation (proxyOwnPropertyKeys, task_3c1fe018) picks up the
// array's symbol via the newly-fixed Object.getOwnPropertySymbols path
// transitively. ---
{
  const target5: any = [1, 2, 3];
  const sym5 = Symbol("s5");
  target5[sym5] = 5;
  const p5 = new Proxy(target5, {});
  const keys5 = Reflect.ownKeys(p5);
  const syms5 = Object.getOwnPropertySymbols(p5);
  checks.push(
    keys5.length === 5 &&
      keys5[keys5.length - 1] === sym5 &&
      syms5.length === 1 &&
      syms5[0] === sym5
  );
}

// --- 6. A symbol property does not leak into Object.getOwnPropertyNames
// or show up as an enumerable string key (sanity: the two enumerators
// stay properly partitioned by key type). ---
{
  const arr6: any = [1, 2];
  const sym6 = Symbol("s6");
  arr6[sym6] = "hidden";
  const names6 = Object.getOwnPropertyNames(arr6);
  checks.push(names6.indexOf(sym6 as any) === -1 && names6.length === 3);
}

// --- 7. Object.getOwnPropertyDescriptor agrees with `arr[sym]` and
// Reflect.get for the same array symbol property - all three read paths
// (bytecode opGetPropSymbol, Reflect.get's Go-side property lookup, and
// this function's own TypeArray branch) had independently drifted before
// this fix; this pins all three at once so they can't silently
// re-diverge. ---
{
  const arr7a: any = [1, 2, 3];
  const sym7a = Symbol("x");
  arr7a[sym7a] = 42;
  const desc = Object.getOwnPropertyDescriptor(arr7a, sym7a) as any;
  checks.push(
    arr7a[sym7a] === 42 &&
      Reflect.get(arr7a, sym7a) === 42 &&
      sym7a in arr7a &&
      desc !== undefined &&
      desc.value === 42 &&
      desc.writable === true &&
      desc.enumerable === true &&
      desc.configurable === true
  );
}

// --- 8. Reading an array's own symbol property takes precedence over an
// identically-keyed property on Array.prototype (opGetPropSymbol's
// TypeArray case used to skip straight to the prototype chain without
// ever consulting the array's own symbolProps at all, so this would have
// found the prototype's value, or undefined, instead of the array's
// own). ---
{
  const sym7 = Symbol("s7");
  (Array.prototype as any)[sym7] = "from-prototype";
  const arr7: any = [1, 2];
  arr7[sym7] = "own-value";
  const readBack = arr7[sym7];
  // Array.prototype is a PlainObject, not an ArrayObject - its
  // DeleteSymbolProp actually removes the entry (verified against Node;
  // this is unrelated to ArrayObject.DeleteSymbolProp's own bug, which
  // is never reached from the `delete` opcode at all - see the deferred
  // follow-up chip), so cleaning up this way doesn't leak the symbol
  // onto the shared intrinsic for later scripts. Assert the cleanup
  // itself worked, not just trust it.
  delete (Array.prototype as any)[sym7];
  const cleanedUp = !(sym7 in (Array.prototype as any));
  checks.push(readBack === "own-value" && cleanedUp);
}

checks.every((c) => c === true);

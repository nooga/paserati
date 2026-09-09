// expect: true
// Object.defineProperty(arr, sym, {...}) was a silent no-op for arrays: it
// neither threw nor stored anything, for both data and accessor
// descriptors, on any array-typed target:
//
//   const arr = [1, 2]; const s = Symbol("d");
//   Object.defineProperty(arr, s, {value: 7, writable: false, enumerable: false, configurable: false});
//   arr[s];                                    // before: undefined - Node: 7
//   Object.getOwnPropertyDescriptor(arr, s);    // before: undefined - Node: {value:7,writable:false,enumerable:false,configurable:false}
//   Object.getOwnPropertySymbols(arr).length;   // before: 0 - Node: 1
//
//   const a2 = [1, 2]; const s2 = Symbol("g");
//   Object.defineProperty(a2, s2, { get() { return 99; } });
//   a2[s2];                                     // before: undefined - Node: 99
//
// Root cause: ArrayObject's existing accessor/attribute-override storage
// (getters/setters/propertyDesc, pkg/vm/value.go) is keyed by string only -
// unlike symbolProps (map[*SymbolObject]Value), which stores a bare value
// with no attribute tracking at all. objectDefinePropertyWithVM
// (pkg/builtins/object_init.go) never had a symbol-key branch for TypeArray
// either - a symbol key fell through every propName-gated check (all
// meaningless for a symbol, since propName is "" for one) and landed on the
// TypeObject-only block below, which a TypeArray value never satisfies.
//
// Fixed by adding symbol-keyed counterparts of the existing string-keyed
// storage - symbolPropertyDesc/symbolGetters/symbolSetters on ArrayObject -
// plus ArrayDefineOwnSymbolProperty (pkg/vm/array_props.go, mirroring
// ArrayDefineOwnProperty's non-index tail) to implement
// [[DefineOwnProperty]] for a symbol key, and wiring it into
// objectDefinePropertyWithVM, Object.getOwnPropertyDescriptor's TypeArray
// branch, and the read (opGetPropSymbol) / write (opSetPropSymbol) bytecode
// paths so an accessor's getter/setter actually fires.
//
// This also surfaced (and fixes) a second, independent gap: `delete
// arr[sym]` (OpDeleteIndex's TypeArray case, pkg/vm/vm.go) unconditionally
// reported success without ever calling ArrayObject.DeleteSymbolProp,
// relying on the old invariant that every symbol property was implicitly
// configurable (true before this fix, since Object.defineProperty could
// never make one otherwise). Now that a symbol property can be
// non-configurable, or an accessor, `delete` is routed through
// DeleteSymbolProp so it both respects configurability and actually clears
// accessor/descriptor state on success - see checks 5-6 below. Same gap,
// same fix, for Reflect.deleteProperty (pkg/builtins/reflect_delete.go).
//
// Every check below (including exact attribute values, TypeError on an
// illegal redefinition, and delete's success/failure split) was verified
// against real Node.js output.

const checks: boolean[] = [];

// --- 1. Data descriptor, all attributes false: round-trips through
// arr[sym], Object.getOwnPropertyDescriptor, and Object.getOwnPropertySymbols. ---
{
  const arr: any = [1, 2];
  const s = Symbol("d");
  Object.defineProperty(arr, s, {
    value: 7,
    writable: false,
    enumerable: false,
    configurable: false,
  });
  const desc = Object.getOwnPropertyDescriptor(arr, s) as any;
  const syms = Object.getOwnPropertySymbols(arr);
  checks.push(
    arr[s] === 7 &&
      desc !== undefined &&
      desc.value === 7 &&
      desc.writable === false &&
      desc.enumerable === false &&
      desc.configurable === false &&
      syms.length === 1 &&
      syms[0] === s
  );
}

// --- 2. Data descriptor, all attributes true (the ordinary-property
// default) - distinguishes "attributes are tracked at all" from "tracked
// attributes happen to read back as their zero value". ---
{
  const arr2: any = [1, 2];
  const s2 = Symbol("all-true");
  Object.defineProperty(arr2, s2, {
    value: "v",
    writable: true,
    enumerable: true,
    configurable: true,
  });
  const desc2 = Object.getOwnPropertyDescriptor(arr2, s2) as any;
  checks.push(
    arr2[s2] === "v" &&
      desc2.writable === true &&
      desc2.enumerable === true &&
      desc2.configurable === true
  );
}

// --- 3. Data descriptor, a mixed combination (writable, non-enumerable,
// configurable) - the two attributes above aren't just being ORed/ANDed
// together into one bit. ---
{
  const arr3: any = [1, 2];
  const s3 = Symbol("mixed");
  Object.defineProperty(arr3, s3, {
    value: 1,
    writable: true,
    enumerable: false,
    configurable: true,
  });
  const desc3 = Object.getOwnPropertyDescriptor(arr3, s3) as any;
  checks.push(
    desc3.writable === true &&
      desc3.enumerable === false &&
      desc3.configurable === true
  );
}

// --- 4. Accessor property: `arr[sym]` invokes the getter (not a stored
// value), and the getter is called freshly on every read. ---
{
  const arr4: any = [1, 2];
  const s4 = Symbol("g");
  let calls = 0;
  Object.defineProperty(arr4, s4, {
    get() {
      calls++;
      return 99;
    },
    enumerable: true,
    configurable: true,
  });
  const first = arr4[s4];
  const second = arr4[s4];
  const desc4 = Object.getOwnPropertyDescriptor(arr4, s4) as any;
  checks.push(
    first === 99 &&
      second === 99 &&
      calls === 2 &&
      typeof desc4.get === "function" &&
      desc4.set === undefined &&
      desc4.enumerable === true &&
      desc4.configurable === true &&
      !("value" in desc4) &&
      !("writable" in desc4)
  );
}

// --- 5. Accessor property with both get and set: assignment invokes the
// setter (not overwriting a plain data slot), and the getter reflects
// whatever the setter stored. ---
{
  const arr5: any = [1, 2];
  const s5 = Symbol("gs");
  let backing = 0;
  Object.defineProperty(arr5, s5, {
    get() {
      return backing;
    },
    set(v: number) {
      backing = v * 2;
    },
    configurable: true,
  });
  arr5[s5] = 21;
  checks.push(arr5[s5] === 42 && backing === 42);
}

// --- 6. Object.getOwnPropertySymbols / Reflect.ownKeys include a symbol
// defined via Object.defineProperty (not just via a plain `arr[sym] = v`
// assignment - the array_getownpropertysymbols.ts test already covers that
// path; this one exercises the descriptor-based path added by this fix). ---
{
  const arr6: any = [1, 2, 3];
  const s6 = Symbol("via-define");
  Object.defineProperty(arr6, s6, { value: "x", enumerable: true });
  const syms6 = Object.getOwnPropertySymbols(arr6);
  const keys6 = Reflect.ownKeys(arr6);
  checks.push(
    syms6.length === 1 &&
      syms6[0] === s6 &&
      keys6[keys6.length - 1] === s6
  );
}

// --- 7. A non-configurable symbol property rejects a defineProperty call
// that tries to widen configurable/enumerable or change writable - and
// `delete arr[sym]` reports failure and leaves the property in place
// (exercises the OpDeleteIndex fix: it used to unconditionally report
// success without even checking, back when every symbol property was
// implicitly configurable). ---
{
  const arr7: any = [1, 2];
  const s7 = Symbol("locked");
  Object.defineProperty(arr7, s7, {
    value: 1,
    writable: false,
    enumerable: false,
    configurable: false,
  });
  let threw = false;
  try {
    Object.defineProperty(arr7, s7, { configurable: true });
  } catch (e) {
    threw = e instanceof TypeError;
  }
  const deleteResult = delete arr7[s7];
  const stillHasIt = s7 in arr7;
  const stillReadsAsOne = arr7[s7] === 1;
  checks.push(threw && deleteResult === false && stillHasIt && stillReadsAsOne);
}

// --- 8. A configurable accessor property CAN be deleted, and deleting it
// actually removes the property (not just reporting success while leaving
// the getter/setter behind) - `sym in arr` and getOwnPropertyDescriptor
// both agree it's gone afterward. ---
{
  const arr8: any = [1, 2];
  const s8 = Symbol("removable");
  Object.defineProperty(arr8, s8, {
    get() {
      return 5;
    },
    configurable: true,
  });
  const deleteResult = delete arr8[s8];
  checks.push(
    deleteResult === true &&
      !(s8 in arr8) &&
      Object.getOwnPropertyDescriptor(arr8, s8) === undefined &&
      Object.getOwnPropertySymbols(arr8).length === 0
  );
}

// --- 9. Reflect.deleteProperty agrees with the `delete` operator for a
// non-configurable symbol property on an array (same underlying
// DeleteSymbolProp fix, different call site - pkg/builtins/reflect_delete.go). ---
{
  const arr9: any = [1, 2];
  const s9 = Symbol("reflect-locked");
  Object.defineProperty(arr9, s9, { value: 1, configurable: false });
  const result9 = Reflect.deleteProperty(arr9, s9);
  checks.push(result9 === false && s9 in arr9);
}

// --- 10. Reflect.get(arr, sym) agrees with `arr[sym]` for a symbol
// accessor - the accessor's getter must fire either way, and both with a
// default receiver and an explicit one (called as `this`). A sibling read
// path (getSymbolPropertyWithReceiver, pkg/vm/vm_init.go) backs
// Reflect.get and had its own, independent no-accessor-check gap - the
// same class of bug array_getownpropertysymbols.ts's header describes
// (opGetPropSymbol vs. Reflect.get drifting apart), just for the
// accessor case this fix newly makes possible. ---
{
  const arr10: any = [1, 2];
  const s10 = Symbol("g10");
  const receiverProbe = { tag: "probe" };
  Object.defineProperty(arr10, s10, {
    get() {
      return (this as any) === receiverProbe ? "receiver" : "self";
    },
    configurable: true,
  });
  checks.push(
    arr10[s10] === "self" &&
      Reflect.get(arr10, s10) === "self" &&
      Reflect.get(arr10, s10, receiverProbe) === "receiver"
  );
}

// --- 11. Object.getOwnPropertyDescriptors (plural) includes a symbol
// property on an array, agreeing with the singular
// Object.getOwnPropertyDescriptor for the same key - the plural function
// collects its own key list per source kind and had no symbol-key
// collection for TypeArray at all, so it silently dropped every array
// symbol property regardless of how it was defined. ---
{
  const arr11: any = [1, 2];
  const s11 = Symbol("plural");
  Object.defineProperty(arr11, s11, {
    value: 3,
    writable: false,
    enumerable: false,
    configurable: false,
  });
  const all = Object.getOwnPropertyDescriptors(arr11) as any;
  const single = Object.getOwnPropertyDescriptor(arr11, s11) as any;
  checks.push(
    all[s11] !== undefined &&
      all[s11].value === 3 &&
      all[s11].writable === false &&
      all[s11].enumerable === false &&
      all[s11].configurable === false &&
      all[s11].value === single.value &&
      all[s11].writable === single.writable
  );
}

checks.every((c) => c === true);

// expect: true
// Reflect.set(target, key, value [, receiver]) unconditionally stringified
// `key` via key.ToString() before ever reaching this file's dispatch - a
// Symbol key silently became its string form ("Symbol(...)") on EVERY
// target kind (plain object, array, Map/Set/RegExp/callable side-tables,
// Proxy), unlike Reflect.get and Reflect.has, which both already
// dispatched on key.Type() === TypeSymbol:
//
//   const o = {}; const s = Symbol("k");
//   console.log(Reflect.set(o, s, 5)); // paserati: true, Node: true (same)
//   console.log(o[s]);                 // before: undefined - Node: 5
//
// Fixed by threading a parallel symbol-key path through Reflect.set's
// entire dispatch chain (pkg/builtins/reflect_init.go):
// reflectSetDispatchByKey -> reflectProxySetByKey / reflectOrdinarySetByKey
// -> reflectCreateOrUpdateDataPropertyByKey / reflectProxyDefineDataPropertyByKey
// - mirroring the string-key chain already there, one function at a time.
// Building the Proxy path fresh also incidentally fixed a second,
// previously-noted-but-not-fixed bug: reflectProxySet's string-key path
// stringifies a Symbol key before handing it to a `set` trap (see that
// function's own doc comment); the new symbol-key path passes the real
// Symbol value straight through, per ECMA-262 10.5.9 step 8.
//
// Every check below was verified against real Node.js output.

const checks: boolean[] = [];

// --- 1. The exact reported repro: a plain object, no existing property. ---
{
  const o: any = {};
  const s = Symbol("k");
  const ok = Reflect.set(o, s, 5);
  checks.push(ok === true && o[s] === 5);
}

// --- 2. A plain object with an EXISTING symbol-keyed accessor: the
// setter must fire (not silently create/overwrite a data slot underneath
// it), and the getter reflects whatever the setter stored. ---
{
  const o2: any = {};
  const s2 = Symbol("acc");
  let backing = 0;
  Object.defineProperty(o2, s2, {
    get() {
      return backing;
    },
    set(v: number) {
      backing = v * 2;
    },
  });
  const ok = Reflect.set(o2, s2, 10);
  checks.push(ok === true && backing === 20 && o2[s2] === 20);
}

// --- 3. A getter-only symbol accessor rejects the write (returns false,
// doesn't throw - Reflect.set never throws for an ordinary failure). ---
{
  const o3: any = {};
  const s3 = Symbol("getonly");
  Object.defineProperty(o3, s3, {
    get() {
      return 7;
    },
    configurable: true,
  });
  const ok = Reflect.set(o3, s3, 100);
  checks.push(ok === false && o3[s3] === 7);
}

// --- 4. An array's plain symbol-keyed data property (no prior
// Object.defineProperty - just Reflect.set creating a brand-new one). ---
{
  const arr: any = [1, 2];
  const s4 = Symbol("d");
  const ok = Reflect.set(arr, s4, 42);
  checks.push(ok === true && arr[s4] === 42);
}

// --- 5. An array with a symbol-keyed accessor defined via
// Object.defineProperty: Reflect.set must invoke the setter, exactly like
// the plain-object accessor case (check 2), not fall back to the array's
// bare symbolProps slot underneath it. ---
{
  const arr2: any = [1, 2];
  const s5 = Symbol("arracc");
  let backing2 = 0;
  Object.defineProperty(arr2, s5, {
    get() {
      return backing2;
    },
    set(v: number) {
      backing2 = v + 1;
    },
    configurable: true,
  });
  const ok = Reflect.set(arr2, s5, 10);
  checks.push(ok === true && backing2 === 11 && arr2[s5] === 11);
}

// --- 6. A non-configurable, non-writable symbol-keyed data property on
// an array rejects the write. ---
{
  const arr3: any = [1, 2];
  const s6 = Symbol("locked");
  Object.defineProperty(arr3, s6, {
    value: 1,
    writable: false,
    configurable: false,
  });
  const ok = Reflect.set(arr3, s6, 2);
  checks.push(ok === false && arr3[s6] === 1);
}

// --- 7. A Proxy WITH a `set` trap: the trap receives the REAL Symbol
// value as its key argument (not a stringified "Symbol(...)"), the
// receiver argument is the proxy itself (default receiver), and the trap
// runs instead of writing straight to the target. ---
{
  const target: any = {};
  const s7 = Symbol("trapped");
  let sawRealSymbol = false;
  let sawCorrectReceiver = false;
  const p: any = new Proxy(target, {
    set(t: any, key: any, value: any, receiver: any) {
      sawRealSymbol = key === s7;
      sawCorrectReceiver = receiver === p;
      t[key] = value;
      return true;
    },
  });
  const ok = Reflect.set(p, s7, 99);
  checks.push(
    ok === true && sawRealSymbol && sawCorrectReceiver && target[s7] === 99
  );
}

// --- 8. A Proxy WITHOUT a `set` trap: per spec, delegates to
// target.[[Set]] - the write lands on the underlying target. ---
{
  const target2: any = {};
  const s8 = Symbol("untrapped");
  const p2 = new Proxy(target2, {});
  const ok = Reflect.set(p2, s8, 55);
  checks.push(ok === true && target2[s8] === 55);
}

// --- 9. A Proxy `set` trap that returns falsy is honored as a rejection
// (Reflect.set reports false), matching the string-key Proxy path. ---
{
  const target3: any = {};
  const s9 = Symbol("refused");
  const p3 = new Proxy(target3, {
    set() {
      return false;
    },
  });
  const ok = Reflect.set(p3, s9, 1);
  checks.push(ok === false && !(s9 in target3));
}

// --- 10. Receiver distinct from target, plain object: the write lands
// on `receiver`, not `target` - same 10.1.9.2 CreateDataProperty(Receiver,
// ...) rule the string-key path already implements (see
// reflect_set_receiver_and_accessors.ts), now exercised for a symbol key. ---
{
  const target4: any = {};
  const receiver4: any = {};
  const s10 = Symbol("recv");
  const ok = Reflect.set(target4, s10, 1, receiver4);
  checks.push(
    ok === true && receiver4[s10] === 1 && !(s10 in target4)
  );
}

// --- 11. An INHERITED symbol-keyed accessor (found by walking target's
// prototype chain, not target's own property) still has its setter
// invoked with the RECEIVER as `this` (not wherever in the chain the
// accessor was found) - this is the actual reason
// reflectOrdinarySetByKey walks a chain at all rather than only checking
// target's own property; every check above exits that loop on its first
// iteration and never exercises the walk or the setter's `this` binding. ---
{
  const s11 = Symbol("inh");
  const proto: any = {};
  let seenThis: any = null;
  let backing3 = 0;
  Object.defineProperty(proto, s11, {
    get() {
      return backing3;
    },
    set(v: number) {
      seenThis = this;
      backing3 = v * 3;
    },
    configurable: true,
  });
  const child: any = Object.create(proto);
  const ok = Reflect.set(child, s11, 4);
  checks.push(
    ok === true &&
      backing3 === 12 &&
      seenThis === child &&
      Object.getOwnPropertySymbols(child).length === 0
  );
}

// --- 12. An INHERITED writable symbol-keyed DATA property: the write
// still lands on the RECEIVER as a new own property (10.1.9.2 step 4),
// never mutating the prototype it was found on - the data-property
// counterpart of check 11, also only reachable via the chain walk. ---
{
  const s12 = Symbol("inhdata");
  const proto2: any = {};
  proto2[s12] = 1;
  const child2: any = Object.create(proto2);
  const ok = Reflect.set(child2, s12, 9);
  checks.push(
    ok === true &&
      child2[s12] === 9 &&
      proto2[s12] === 1 &&
      Object.getOwnPropertySymbols(child2).length === 1
  );
}

checks.every((c) => c === true);

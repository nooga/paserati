// expect: true
// Reflect.set's "set" closure (pkg/builtins/reflect_init.go) used to have
// a "Simple property set on target" fallback that unconditionally called
// target.AsPlainObject().SetOwn(...) / target.AsDictObject().SetOwn(...) /
// arr.Set(idx, ...) directly, regardless of:
//
//  - `receiver` being distinct from `target` (a Proxy receiver was
//    special-cased above it; any OTHER kind of distinct receiver - the
//    common case, a plain object - fell straight through to the
//    always-writes-to-target fallback), and
//  - `target` (or its prototype chain) having an own or inherited
//    accessor at all - the fallback never checked, so even the ordinary
//    receiver === target form silently clobbered/no-opped an existing
//    setter instead of calling it.
//
// Both are the same root cause (the fallback had no descriptor awareness
// whatsoever) and are fixed together by reflectOrdinarySet, a real
// implementation of ECMA-262 10.1.9 OrdinarySet / 10.1.9.2
// OrdinarySetWithOwnDescriptor: walk target's own property, then its
// whole [[Prototype]] chain, for the first applicable descriptor - an
// accessor's setter is invoked with `receiver` as `this` wherever in the
// chain it's found; a data descriptor's actual write always lands on
// `receiver` (via reflectCreateOrUpdateDataProperty), never on whichever
// object in target's chain the descriptor happened to be found on.
//
// Every check here was verified against real Node.js output.

const checks: boolean[] = [];

// --- 1. Distinct plain-object receiver, property absent on target and
// its whole prototype chain: per 10.1.9.2's final
// CreateDataProperty(Receiver, ...) step, the write lands on `receiver`,
// not `target` - the core bug this fix closes. ---
{
  const target: any = {};
  const receiver: any = {};
  const ok = Reflect.set(target, "y", 5, receiver);
  checks.push(ok === true && receiver.y === 5 && !("y" in target));
}

// --- 2. receiver === target (the common/simple form) must not regress. ---
{
  const target2: any = {};
  const ok = Reflect.set(target2, "x", 1, target2);
  checks.push(ok === true && target2.x === 1);
}

// --- 3. receiver already has its own (writable, non-accessor) property
// for the same key: the write updates the RECEIVER's existing property in
// place (not the target, which still must not gain the key at all). ---
{
  const target3: any = {};
  const receiver3: any = { y: 10 };
  const ok = Reflect.set(target3, "y", 99, receiver3);
  checks.push(ok === true && receiver3.y === 99 && !("y" in target3));
}

// --- 4. Inherited accessor (setter defined on target's prototype, not
// target itself) with a distinct receiver: the setter still fires, and
// `this` inside it is the RECEIVER, not target or the prototype - so the
// side effect the setter performs (`this._z = v`) lands on the receiver. ---
{
  const proto: any = {};
  Object.defineProperty(proto, "z", {
    get() {
      return 2;
    },
    set(v: any) {
      (this as any)._z = v;
    },
    configurable: true,
  });
  const child: any = Object.create(proto);
  const rec: any = {};
  const ok = Reflect.set(child, "z", 42, rec);
  checks.push(ok === true && rec._z === 42 && (child as any)._z === undefined);
}

// --- 5. Own accessor, receiver === target (the second bug found while
// fixing check 1 - not previously covered by any test at all): the
// setter must actually be invoked, with `this` bound to the object,
// instead of being silently bypassed. ---
{
  const obj: any = {};
  let calls = 0;
  let lastValue: any = null;
  Object.defineProperty(obj, "y", {
    get() {
      return 1;
    },
    set(v: any) {
      calls++;
      lastValue = v;
    },
    configurable: true,
  });
  const ok = Reflect.set(obj, "y", 7);
  checks.push(ok === true && calls === 1 && lastValue === 7 && obj.y === 1);
}

// --- 6. An accessor with no setter (getter-only) refuses the write and
// returns false, rather than silently succeeding or throwing. ---
{
  const o: any = {};
  Object.defineProperty(o, "y", {
    get() {
      return 1;
    },
    configurable: true,
  });
  const ok = Reflect.set(o, "y", 5);
  checks.push(ok === false);
}

// --- 7. A non-writable, non-accessor data property on target refuses the
// write and returns false. ---
{
  const o2: any = Object.freeze({ x: 1 });
  const ok = Reflect.set(o2, "x", 2);
  checks.push(ok === false && o2.x === 1);
}

// --- 8. A primitive receiver (not an object) makes the data-write branch
// fail with false, per 10.1.9.2 step 4.a - not a throw. ---
{
  const ok = Reflect.set({}, "y", 5, 42 as any);
  checks.push(ok === false);
}

// --- 9. receiver has its own NON-WRITABLE property for the key: the
// write must be refused (false), and the receiver's value must not
// change, even though target has no such restriction at all. ---
{
  const target4: any = {};
  const receiver4: any = {};
  Object.defineProperty(receiver4, "y", { value: 1, writable: false, configurable: true });
  const ok = Reflect.set(target4, "y", 99, receiver4);
  checks.push(ok === false && receiver4.y === 1);
}

// --- 10. receiver has its own ACCESSOR for the key: a plain data write
// can't overwrite an accessor, so this must be refused (false) and the
// receiver's setter must NOT be invoked (a data write is not the same
// operation as calling the accessor). ---
{
  const target5: any = {};
  const receiver5: any = {};
  Object.defineProperty(receiver5, "y", {
    get() {
      return 1;
    },
    set(v: any) {
      (this as any)._y = v;
    },
    configurable: true,
  });
  const ok = Reflect.set(target5, "y", 99, receiver5);
  checks.push(ok === false && receiver5._y === undefined);
}

// --- 11. Array "length" actually resizes (the pre-existing fallback had
// an explicit "Setting length is complex, skip for now" no-op stub here -
// verified against Node, which does resize). ---
{
  const arr: any = [1, 2, 3, 4, 5];
  const ok = Reflect.set(arr, "length", 2);
  checks.push(ok === true && arr.length === 2 && arr[0] === 1 && arr[1] === 2 && arr[2] === undefined);
}

// --- 12. Array named (non-index) property still works, using a distinct
// array from check 11 per this session's "distinct target per assertion"
// convention for shared/mutable state. ---
{
  const arr2: any = [1, 2, 3];
  const ok = Reflect.set(arr2, "customProp", 5);
  checks.push(ok === true && arr2.customProp === 5);
}

// --- 13. Array numeric-index set still works. ---
{
  const arr3: any = [1, 2, 3];
  const ok = Reflect.set(arr3, "1", 99);
  checks.push(ok === true && arr3[1] === 99);
}

checks.every((c) => c === true);

// expect: true
// objectGetOwnPropertyDescriptorWithVM's TypeBoundFunction case and its
// TypeRegExp block (pkg/builtins/object_init.go) both looked up an own
// property by string name only (propName), never checking keyIsSymbol or
// using propSym - unlike the sibling TypeMap/TypeSet/TypePromise block in
// the same function, which correctly branches on keyIsSymbol and uses
// GetOwnAccessorByKey/GetOwnDescriptorByKey for a symbol key. So
// Object.getOwnPropertyDescriptor(regexOrBoundFn, sym) always answered
// undefined for a real own symbol property, even though `in`/Reflect.has
// already found it correctly.
const checks: boolean[] = [];

function hasDataDesc(
  desc: any,
  value: unknown,
  writable: boolean,
  enumerable: boolean,
  configurable: boolean
): boolean {
  return (
    desc !== undefined &&
    desc.value === value &&
    desc.writable === writable &&
    desc.enumerable === enumerable &&
    desc.configurable === configurable
  );
}

// --- RegExp: an own symbol property set via bracket-notation assignment ---
const sym = Symbol("k");
const r: any = /x/g;
r[sym] = 4;
checks.push(hasDataDesc(Object.getOwnPropertyDescriptor(r, sym), 4, true, true, true));
checks.push(sym in r && Reflect.has(r, sym));

// --- RegExp: a genuinely absent symbol key ---
checks.push(Object.getOwnPropertyDescriptor(r, Symbol("absent")) === undefined);

// --- RegExp: "lastIndex" (a real own property, but never a symbol) is
// unaffected by routing symbol keys to the side table first ---
const lastIndexDesc: any = Object.getOwnPropertyDescriptor(r, "lastIndex");
checks.push(hasDataDesc(lastIndexDesc, 0, true, false, false));

// --- RegExp: a symbol-keyed accessor (set via Object.defineProperty,
// since bracket-assignment through an accessor has its own separate,
// unrelated pre-existing gap) ---
const r2: any = /y/g;
Object.defineProperty(r2, sym, {
  get() {
    return 42;
  },
  enumerable: true,
  configurable: true,
});
const r2Desc: any = Object.getOwnPropertyDescriptor(r2, sym);
checks.push(
  r2Desc !== undefined &&
    typeof r2Desc.get === "function" &&
    r2Desc.get() === 42 &&
    r2Desc.set === undefined &&
    r2Desc.enumerable === true &&
    r2Desc.configurable === true
);

// --- BoundFunction: an own symbol property set via bracket-notation
// assignment. Only Reflect.has is checked (not `in`): the `in` operator's
// symbol-key dispatch has its own, separate, still-open gap for
// BoundFunction (tracked independently) - unrelated to this
// getOwnPropertyDescriptor fix, which this test is actually about. ---
function fn() {}
const bound: any = fn.bind(null);
bound[sym] = 5;
checks.push(hasDataDesc(Object.getOwnPropertyDescriptor(bound, sym), 5, true, true, true));
checks.push(Reflect.has(bound, sym));

// --- BoundFunction: a genuinely absent symbol key ---
checks.push(Object.getOwnPropertyDescriptor(bound, Symbol("absent")) === undefined);

// --- BoundFunction: "name"/"length" (real own properties, but never a
// symbol) are unaffected ---
const boundNameDesc: any = Object.getOwnPropertyDescriptor(bound, "name");
checks.push(
  boundNameDesc !== undefined &&
    boundNameDesc.value === "bound fn" &&
    boundNameDesc.writable === false &&
    boundNameDesc.enumerable === false &&
    boundNameDesc.configurable === true
);

// --- BoundFunction: a symbol-keyed accessor ---
const bound2: any = fn.bind(null);
Object.defineProperty(bound2, sym, {
  get() {
    return 43;
  },
  enumerable: true,
  configurable: true,
});
const bound2Desc: any = Object.getOwnPropertyDescriptor(bound2, sym);
checks.push(
  bound2Desc !== undefined &&
    typeof bound2Desc.get === "function" &&
    bound2Desc.get() === 43 &&
    bound2Desc.set === undefined &&
    bound2Desc.enumerable === true &&
    bound2Desc.configurable === true
);

checks.every((c) => c === true);

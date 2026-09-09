// expect: true
// Object.defineProperty threw "Object.defineProperty called on non-object"
// on a plain TypeNativeFunction value (e.g. Array.prototype.push), even
// though the exact same value already accepted a bracket-notation
// assignment fine, and every other callable kind (TypeFunction,
// TypeClosure, TypeBoundFunction, TypeNativeFunctionWithProps) was already
// accepted - pkg/builtins/object_init.go's isObjectLike gate near the top
// of objectDefinePropertyWithVM simply never listed TypeNativeFunction,
// even though its Properties *PlainObject side table (pkg/vm/function.go)
// is identical in shape to the other four callable kinds.
//
// Fixing the write side surfaced two read-side siblings that also never
// checked a plain native function's Properties table at all:
//   - objectGetOwnPropertyDescriptorWithVM's TypeNativeFunction case only
//     ever answered "name"/"length" - defining anything else was invisible
//     to Object.getOwnPropertyDescriptor.
//   - OpIn's non-symbol TypeNativeFunction case (pkg/vm/vm.go) carried a
//     comment claiming "native functions don't have custom properties" and
//     skipped straight to the FunctionPrototype fallback, so `"x" in nf`
//     disagreed with Reflect.has(nf, "x") right after defining "x".
// All three are fixed here, in the same commit as the write-side fix -
// same "don't ship half a causally-coupled fix" reasoning as PR #332.
//
// Each assertion below uses its own function (Array.prototype methods are
// real shared intrinsics - a distinct one per check keeps one assertion's
// mutation from being observable by another).

const checks: boolean[] = [];

function hasDataDesc(desc: any, value: unknown, writable: boolean, enumerable: boolean, configurable: boolean): boolean {
  return (
    desc !== undefined &&
    desc.value === value &&
    desc.writable === writable &&
    desc.enumerable === enumerable &&
    desc.configurable === configurable
  );
}

// --- A data property via Object.defineProperty on a plain native function,
// read back via Object.getOwnPropertyDescriptor (singular) and direct
// property access. ---
{
  const nf: any = Array.prototype.push;
  Object.defineProperty(nf, "tag", { value: 1, writable: true, enumerable: true, configurable: true });
  checks.push(nf.tag === 1);
  checks.push(hasDataDesc(Object.getOwnPropertyDescriptor(nf, "tag"), 1, true, true, true));
}

// --- A symbol-keyed data property, same as above. Verified only via
// Object.getOwnPropertyDescriptor, not `nf[sym]`: reading a symbol-keyed
// property back via bracket notation on a plain native function has its
// own separate, pre-existing GET-side gap (opGetPropSymbol has no
// TypeNativeFunction case at all) - flagged independently, not fixed
// here. ---
{
  const nf: any = Array.prototype.pop;
  const sym = Symbol("k");
  Object.defineProperty(nf, sym, { value: 2, writable: true, enumerable: true, configurable: true });
  checks.push(hasDataDesc(Object.getOwnPropertyDescriptor(nf, sym), 2, true, true, true));
}

// --- An accessor property: the setter actually runs (verified via a
// captured side-effect variable, not a read-back). The getter is
// confirmed callable via the descriptor's own `get` function, not via
// `nf.custom` direct read - invoking an own accessor's getter through
// ordinary property access on a plain native function is a second,
// separate pre-existing GET-side gap (opGetProp's TypeNativeFunction
// case only ever calls Properties.GetOwn, which doesn't return an
// accessor's value) - also flagged independently, not fixed here. ---
{
  const nf: any = Array.prototype.shift;
  let backing = 0;
  Object.defineProperty(nf, "custom", {
    get() {
      return backing;
    },
    set(v: number) {
      backing = v;
    },
    enumerable: true,
    configurable: true,
  });
  nf.custom = 100;
  checks.push(backing === 100);
  const desc: any = Object.getOwnPropertyDescriptor(nf, "custom");
  checks.push(typeof desc.get === "function" && desc.get() === 100 && typeof desc.set === "function");
}

// --- name/length intrinsics are preserved: still their synthesized
// {writable: false, enumerable: false, configurable: true} shape, and
// redefining name's *value* (permitted since it's configurable) works. ---
{
  const nf: any = Array.prototype.unshift;
  checks.push(hasDataDesc(Object.getOwnPropertyDescriptor(nf, "name"), "unshift", false, false, true));
  checks.push(
    hasDataDesc(Object.getOwnPropertyDescriptor(nf, "length"), Array.prototype.unshift.length, false, false, true)
  );
  Object.defineProperty(nf, "name", { value: "renamed" });
  checks.push(nf.name === "renamed");
}

// --- `in`/Reflect.has agree for a property just defined via
// Object.defineProperty, both string- and symbol-keyed. ---
{
  const nf: any = Array.prototype.slice;
  Object.defineProperty(nf, "strkey", { value: 1, configurable: true });
  checks.push("strkey" in nf && Reflect.has(nf, "strkey"));
  const sym2 = Symbol("k2");
  Object.defineProperty(nf, sym2, { value: 1, configurable: true });
  checks.push(sym2 in nf && Reflect.has(nf, sym2));
}

// --- Reflect.defineProperty (delegates to the same fixed gate). ---
{
  const nf: any = Array.prototype.concat;
  const ok = Reflect.defineProperty(nf, "viaReflect", { value: "v", writable: true, enumerable: true, configurable: true });
  checks.push(ok === true);
  checks.push(nf.viaReflect === "v");
}

// --- A genuinely absent property still answers undefined/false
// everywhere. ---
{
  const nf: any = Array.prototype.indexOf;
  checks.push(Object.getOwnPropertyDescriptor(nf, "absent") === undefined);
  checks.push(!("absent" in nf) && !Reflect.has(nf, "absent"));
}

// --- Non-extensible rejects a new property (must be the LAST assertion
// touching this particular function, since preventExtensions is permanent
// for the life of this VM instance). ---
{
  const nf: any = Array.prototype.lastIndexOf;
  Object.preventExtensions(nf);
  let threw = false;
  try {
    Object.defineProperty(nf, "z", { value: 1, configurable: true });
  } catch (e) {
    threw = e instanceof TypeError;
  }
  checks.push(threw);
}

checks.every((c) => c === true);

// expect: true
// OpIn's symbol-key switch (pkg/vm/vm.go, `if propVal.Type() == TypeSymbol`
// then `switch objVal.Type()`) had cases for TypeObject, TypeDictObject,
// TypeArray, TypeSet, TypeMap, TypeArguments, TypePromise, TypeRegExp,
// TypeFunction, and TypeClosure - but nothing for TypeBoundFunction,
// TypeNativeFunction, or TypeNativeFunctionWithProps, so a symbol-keyed `in`
// check on any of those three fell through to `default: hasProperty =
// false`, even for a real own symbol property that Reflect.has (and the
// underlying Properties side table itself) already found correctly:
//
//   const bound = (function () {}).bind(null);
//   bound[Symbol("k")] = 99;
//   Reflect.has(bound, sym); // true (correct)
//   sym in bound;            // false (WRONG) - Node: true
//
// Fixed by adding a case for all three, mirroring the existing
// TypeFunction/TypeClosure cases: own Properties table first (via the
// generic OwnPropertiesTable helper, since all three carry one - see
// pkg/vm/properties_table.go's ownPropertiesSlot), then walk
// vm.FunctionPrototype's chain for an inherited symbol property.

const checks: boolean[] = [];

// --- BoundFunction: an own symbol property set via bracket-notation
// assignment must be found by both `in` and Reflect.has. ---
function fn() {}
const bound: any = fn.bind(null);
const sym = Symbol("k");
bound[sym] = 99;
checks.push(sym in bound);
checks.push(Reflect.has(bound, sym));

// --- BoundFunction: a genuinely absent symbol key. ---
checks.push(!(Symbol("absent") in bound));
checks.push(!Reflect.has(bound, Symbol("absent")));

// --- NativeFunctionWithProps (a native constructor, e.g. Array): an own
// symbol property must be found by both. ---
const sym2 = Symbol("k2");
const Arr: any = Array;
Arr[sym2] = 6;
checks.push(sym2 in Arr);
checks.push(Reflect.has(Arr, sym2));

// --- NativeFunctionWithProps: a genuinely absent symbol key. ---
checks.push(!(Symbol("absent2") in Arr));
checks.push(!Reflect.has(Arr, Symbol("absent2")));

// --- NativeFunction (a plain native method): its Properties table isn't
// allocated until the first property is set on it - own symbol property
// must still be found by both afterward. ---
const sym3 = Symbol("k3");
const nf: any = Array.prototype.push;
nf[sym3] = 1;
checks.push(sym3 in nf);
checks.push(Reflect.has(nf, sym3));

// --- NativeFunction: a genuinely absent symbol key on a function whose
// Properties table has never been allocated at all. ---
const nf2: any = Array.prototype.pop;
checks.push(!(Symbol("absent3") in nf2));
checks.push(!Reflect.has(nf2, Symbol("absent3")));

// --- String keys on all three kinds are unaffected by this fix. ---
checks.push("name" in bound && Reflect.has(bound, "name"));
checks.push("name" in Arr && Reflect.has(Arr, "name"));
checks.push("call" in nf && Reflect.has(nf, "call"));

// --- `in`'s FunctionPrototype walk (used above by BoundFunction) also
// correctly finds an *inherited* symbol property, e.g.
// Function.prototype[Symbol.hasInstance], and Reflect.has agrees - both
// now true, matching Node. (This assertion previously pinned a
// false/false agreement caused by two compounding pre-existing bugs: the
// walk itself couldn't find FunctionPrototype's own properties at all,
// since vm.FunctionPrototype is a TypeNativeFunctionWithProps at runtime
// rather than a bare PlainObject, and Reflect.has unconditionally skipped
// the walk for any symbol key. Both are fixed now - see
// hasFunctionPrototypeSymbolProperty's doc comment in pkg/vm/vm.go.) ---
checks.push(Symbol.hasInstance in bound && Reflect.has(bound, Symbol.hasInstance));

// --- Same inherited-symbol lookup for a plain function and a class,
// verified to match Node (both true). ---
function plainFn() {}
checks.push(Symbol.hasInstance in plainFn && Reflect.has(plainFn, Symbol.hasInstance));

class SomeClass {}
checks.push(Symbol.hasInstance in SomeClass && Reflect.has(SomeClass, Symbol.hasInstance));

// --- The walk doesn't stop at Function.prototype - it continues up to
// Function.prototype's own [[Prototype]] (Object.prototype) too, for a
// symbol property that lives there instead. Confirms the fix's shared
// helper (hasFunctionPrototypeSymbolProperty) walks the full chain, not
// just its first step. ---
const inheritedFromObjectProto = Symbol("inheritedFromObjectProto");
(Object.prototype as any)[inheritedFromObjectProto] = 1;
checks.push(
  inheritedFromObjectProto in plainFn && Reflect.has(plainFn, inheritedFromObjectProto)
);

// --- A class that overrides Symbol.hasInstance as its own static
// property is still found via the own-table check, same as before -
// unaffected by the FunctionPrototype walk fix. ---
class OverridesHasInstance {
  static [Symbol.hasInstance](_x: unknown) {
    return true;
  }
}
checks.push(Symbol.hasInstance in OverridesHasInstance && Reflect.has(OverridesHasInstance, Symbol.hasInstance));

checks.every((c) => c === true);

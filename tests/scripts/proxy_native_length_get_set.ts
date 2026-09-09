// expect: true
// Two related bugs in the NATIVE/Go-level property access entry points
// pkg/builtins uses (vm.GetProperty/getPropertyWithReceiver and
// vm.SetProperty, pkg/vm/vm_init.go) - as opposed to the bytecode-compiled
// OpGetProp/OpSetProp paths, which already handled these cases correctly -
// found while regression-testing the pkg/vm Proxy trap-lookup fixes
// earlier in this stack:
//
//   1. vm.SetProperty had NO `case TypeProxy` at all - a TypeProxy `obj`
//      fell straight to `default: return nil` (a silent no-op). Every
//      native Go caller that writes a property via SetProperty on a
//      Proxy did nothing, most visibly
//      pkg/builtins/array_generic.go's arrayLikeSetLength (used by
//      Array.prototype.pop/shift/splice/copyWithin to shrink `.length`
//      after removing an element): `Array.prototype.pop.call(new
//      Proxy(realArray, {}))` deleted the right element but never
//      updated realArray.length, even though ordinary `proxy.length = n`
//      assignment syntax (bytecode OpSetProp, op_setprop.go) already
//      worked correctly for the exact same proxy and key.
//
//   2. op_getprop.go's opGetProp (bytecode OpGetProp, `proxy.prop`) had a
//      "no get trap, fallback to target" branch that only ever handled a
//      TypeObject/TypeDictObject/callable target - every other legal
//      proxy.target kind (TypeArray, TypeMap, TypeSet, TypePromise,
//      TypeRegExp, TypeGenerator, TypeBoundFunction, TypeNativeFunction,
//      TypeNativeFunctionWithProps, TypeArguments, a further-nested
//      Proxy, ...) fell to a bare `*dest = Undefined`. Two helper calls
//      in that branch (handleSpecialProperties, handlePrimitiveMethod)
//      switch on the TARGET's own kind (TypeArray/TypeMap/TypeString/...)
//      but were only ever reached once target.Type() == TypeObject was
//      already established - so neither call could ever actually match
//      anything at that call site; both were dead code there. Most
//      visibly: `new Proxy(realArray, {})` (no get trap, a real Array
//      target) silently read `undefined` for EVERY property, including
//      "length" itself.
//
// Fixed via:
//   - SetProperty gained a `case TypeProxy` mirroring op_setprop.go's own
//     TypeProxy case: GetMethod(handler, "set") via getInheritedGeneric
//     (an inherited trap counts), and a no-trap fallback that recurses
//     into SetProperty on the target - reaching the existing
//     TypeArray/TypeObject/TypeDictObject/TypeArguments/TypeRegExp cases
//     (including "length" on a real Array) automatically, plus a
//     further-nested Proxy target (recursing through this same switch
//     again).
//   - opGetProp's "no get trap" branch now delegates to
//     getPropertyWithReceiver (pkg/vm/vm_init.go) instead of its own
//     partial reimplementation - that function already handles every
//     Value kind (including TypeArray's "length", and recursing through
//     a further-nested Proxy) and already threads the receiver through
//     any accessor/trap found along the way, matching how this same
//     function's get-trap-call branch already does.
//
// Not fixed here (a separate, pre-existing, confirmed-independent bug
// filed as a follow-up): OpGetIndex (bracket notation, `proxy[key]`) has
// its OWN "no get trap, fallback to target" switch with the same
// incompleteness, but for a different opcode - `new Proxy(arguments, {})`
// reads its own `.length` correctly (dot notation, opGetProp, fixed
// above) but `p[0]`/`p[1]` (bracket notation, OpGetIndex) still read
// `undefined`. Not covered by this file.

const checks: boolean[] = [];

// --- 1. Array.prototype.pop via .call() on a plain, trap-less
// array-wrapping Proxy: deletes the right element AND shrinks the real
// array's length (arrayLikeSetLength -> SetProperty -> the new TypeProxy
// case -> the target's own "length" case). Matches Node exactly. ---
{
  const arr: any = [1, 2, 3];
  const p: any = new Proxy(arr, {});
  const popped = Array.prototype.pop.call(p);
  checks.push(popped === 3 && arr.length === 2);
}

// --- 2. Array.prototype.shift via .call(): same shape, a different
// arrayLikeDelete/arrayLikeSetLength call site. ---
{
  const arr: any = [1, 2, 3];
  const p: any = new Proxy(arr, {});
  const shifted = Array.prototype.shift.call(p);
  checks.push(shifted === 1 && arr.length === 2 && arr[0] === 2 && arr[1] === 3);
}

// --- 3. Reading `.length` directly off a trap-less array-wrapping Proxy
// (dot notation, opGetProp's fixed fallback) - was `undefined` before
// this fix, regardless of any prior mutation. ---
{
  const arr: any = [1, 2, 3];
  const p: any = new Proxy(arr, {});
  checks.push(p.length === 3 && p[0] === 1 && p[1] === 2);
}

// --- 4. Same read, immediately after a native-path mutation (pop),
// isolating the get-path fix from the set-path fix (check 1 already
// covers arr.length directly; this covers reading it back through the
// proxy too). ---
{
  const arr: any = [1, 2, 3];
  const p: any = new Proxy(arr, {});
  Array.prototype.pop.call(p);
  checks.push(p.length === 2);
}

// --- 5. `.length` write via ordinary assignment syntax (bytecode
// OpSetProp, already correct before this fix) still agrees with the
// now-fixed native path - not a regression check for something new,
// just confirming both paths land on the same real array state. ---
{
  const arr: any = [1, 2, 3];
  const p: any = new Proxy(arr, {});
  p.length = 1;
  checks.push(arr.length === 1 && p.length === 1);
}

// --- 6. opGetProp's fallback extension beyond TypeArray: RegExp
// lastIndex through a trap-less Proxy (was already reachable via a
// TypeObject-adjacent path before, but exercised here as a second
// non-TypeObject/TypeDictObject/callable target kind, for coverage
// alongside check 3's Array case). ---
{
  const re: any = /a/g;
  const p: any = new Proxy(re, {});
  re.lastIndex = 5;
  checks.push(p.lastIndex === 5);
}

// --- 7. A TypeDictObject handler (a TS enum at runtime) must still not
// panic for either the fixed get or set path. ---
{
  enum DictHandler7 {
    A,
    B,
  }
  const arr: any = [1, 2, 3];
  const p: any = new Proxy(arr, DictHandler7 as any);
  checks.push(p.length === 3);
  const popped = Array.prototype.pop.call(p);
  checks.push(popped === 3 && arr.length === 2);
}

checks.every((c) => c === true);

// expect: true
// Follow-up filed while regression-testing earlier PRs in this stack: an
// audit of pkg/vm/vm.go for the same "no trap, fall back to target
// directly, only handles TypeObject/TypeDictObject" shape that affected
// the `get` trap (OpObjectSpread, opGetProp, OpGetIndex - all fixed by
// prior commits on this branch) turned up SEVEN more sites, all backing
// the `has` trap's no-trap fallback for every `with`-statement opcode:
// OpGetWithProperty, OpSetWithProperty, OpGetWithOrLocal,
// OpSetWithOrLocal, OpResolveWithBinding (the checkWithObjProperty
// closure), OpSetWithByBinding, and OpGetWithByBinding. Each checked
// `proxy.target.Type() == TypeObject` (a couple also TypeDictObject) and
// silently answered "property not present" for any other kind, including
// a further-nested trap-less Proxy target:
//
//   const inner = { x: 5 };
//   const p1 = new Proxy(inner, {});
//   const p2 = new Proxy(p1, {}); // no has trap; p1 has none either
//   function f() {
//     let x = 1;
//     with (p2) { x = 99; } // p2 claims "x" only if it can see inner.x
//     return [x, inner.x];
//   }
//   f(); // was [99, 1] (with-object never claimed the binding) -
//        // now [1, 99], matching Node
//
// Fixed by routing every site through proxyHasPropertyFallback (the
// helper an earlier PR in this stack introduced and already used for
// OpIn's own `has` fallback) instead of each site's own hand-rolled
// TypeObject/TypeDictObject-only check - it already recurses through a
// nested Proxy target (checking that inner proxy's own has trap first)
// and covers several other target kinds these sites didn't (TypeArray,
// TypeMap, TypeSet, TypeRegExp, TypePromise, ...). Two of the seven
// sites (OpSetWithByBinding, OpGetWithByBinding) keep their own
// deliberate "unhandled kind -> assume still bound, don't throw"
// default for SetMutableBinding's step-2 check (a design choice
// predating this fix, not something this fix changes) - only their
// TypeProxy case was added, delegating to proxyHasPropertyFallback for
// that one kind specifically.
//
// Each check below exercises a different one of the seven opcodes,
// verified against real Node output. All use the same shape: a nested,
// fully trap-less Proxy chain wrapping a real object, so `with` can only
// correctly claim the binding by resolving all the way through.

const checks: boolean[] = [];
declare var gRead: any;
declare var gWrite: any;
declare var gCompound: any;

// --- 1. OpGetWithProperty: reading a global variable inside `with`. ---
{
  const inner: any = { gRead: 5 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  (globalThis as any).gRead = 1;
  function readGlobal() {
    with (p2) {
      return gRead;
    }
  }
  checks.push(readGlobal() === 5);
}

// --- 2. OpSetWithProperty: writing a global variable inside `with`. ---
{
  const inner: any = { gWrite: 5 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  (globalThis as any).gWrite = 1;
  function writeGlobal() {
    with (p2) {
      gWrite = 42;
    }
  }
  writeGlobal();
  checks.push((globalThis as any).gWrite === 1 && inner.gWrite === 42);
}

// --- 3. OpGetWithOrLocal: reading a local variable inside `with` - the
// with-object must take priority over the local. ---
{
  const inner: any = { x: 5 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  function f() {
    let x = 1;
    with (p2) {
      return x;
    }
  }
  checks.push(f() === 5);
}

// --- 4. OpSetWithOrLocal: writing a local variable inside `with` - the
// with-object must claim the binding, leaving the local untouched. ---
{
  const inner: any = { x: 5 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  function f() {
    let x = 1;
    with (p2) {
      x = 99;
    }
    return [x, inner.x];
  }
  const [x, ix] = f();
  checks.push(x === 1 && ix === 99);
}

// --- 5-7. OpResolveWithBinding + OpSetWithByBinding + OpGetWithByBinding:
// a compound assignment inside `with`, for both a local and a global
// variable - exercises all three opcodes together (resolving which
// binding to use, then the get-then-set halves of the compound op). ---
{
  const inner: any = { lx: 10 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  function f() {
    let lx = 1;
    with (p2) {
      lx += 5;
    }
    return [lx, inner.lx];
  }
  const [lx, ilx] = f();
  checks.push(lx === 1 && ilx === 15);
}
{
  const inner: any = { gCompound: 10 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  (globalThis as any).gCompound = 1;
  function f() {
    with (p2) {
      gCompound += 5;
    }
  }
  f();
  checks.push((globalThis as any).gCompound === 1 && inner.gCompound === 15);
}

checks.every((c) => c === true);

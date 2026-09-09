// expect: true
// Two bugs shared by every `with`-statement Proxy trap lookup in the
// run() interpreter loop (pkg/vm/vm.go), found while auditing every
// `proxy.handler.AsPlainObject().GetOwn(<trap>)` call site after #342
// fixed this exact pattern for OpIn's `has` lookup:
//
//   1. Own-only trap lookup, not GetMethod-inherited. Each site did
//      `proxy.handler.AsPlainObject().GetOwn(<trap>)` - own-only - instead
//      of GetMethod(handler, <trap>) per spec, which walks the handler's
//      own [[Prototype]] chain. An inherited trap (e.g. `Object.create({
//      has() {...} })` as a handler) was invisible on every `with`-object
//      code path: OpGetWithProperty/OpSetWithProperty (a global variable
//      read/write inside `with`), OpGetWithOrLocal/OpSetWithOrLocal (same,
//      for a variable that's also a local in the current function),
//      OpResolveWithBinding/OpGetWithByBinding/OpSetWithByBinding (a
//      compound assignment's pre-resolved binding), and isUnscopable's own
//      Proxy branch (checking @@unscopables on a `with`-object during any
//      of the above).
//
//   2. Unguarded AsPlainObject() panics on a TypeDictObject handler. Every
//      one of those sites assumed the handler is specifically a
//      TypeObject. A handler that happens to be a TypeDictObject (a TS
//      enum or module namespace value at runtime) made Value.AsPlainObject()
//      panic the entire process - not a catchable JS exception.
//
// Fixed by routing every one of these sites through proxyGetTrap (the
// helper #342 already introduced for OpIn's `has` case), which branches on
// TypeObject/TypeDictObject and uses .Get (not .GetOwn) for both.
//
// checks 1-2: OpGetWithProperty / OpSetWithProperty (global variable).
// checks 3-4: OpGetWithOrLocal / OpSetWithOrLocal (local variable, with
//             the with-object taking priority per `with` semantics - the
//             local must NOT be touched when the with-object claims the
//             binding via an inherited `has`).
// checks 5-6: compound assignment (OpResolveWithBinding + OpGetWithByBinding
//             + OpSetWithByBinding), inherited get/set traps.
// check 7:    a TypeDictObject handler must not panic on any `with` path.

const checks: boolean[] = [];

// --- 1. Global variable read inside `with`, inherited `get`/`has` traps. ---
var wGlobal = 1;
{
  const base = {
    has(_t: any, k: string) {
      return k === "wGlobal";
    },
    get(_t: any, k: string) {
      return k === "wGlobal" ? 111 : undefined;
    },
  };
  const handler: any = Object.create(base);
  function readGlobal() {
    with (new Proxy({}, handler) as any) {
      return wGlobal;
    }
  }
  checks.push(readGlobal() === 111);
}

// --- 2. Global variable write inside `with`, inherited `has`/`set` traps. ---
{
  const log: string[] = [];
  const base = {
    has(_t: any, k: string) {
      log.push("has:" + k);
      return k === "wGlobal";
    },
    set(_t: any, k: string, v: any) {
      log.push("set:" + k + "=" + v);
      return true;
    },
  };
  const handler: any = Object.create(base);
  function writeGlobal() {
    with (new Proxy({}, handler) as any) {
      wGlobal = 222;
    }
  }
  writeGlobal();
  checks.push(log.indexOf("set:wGlobal=222") !== -1);
}

// --- 3. Local variable read inside `with` (OpGetWithOrLocal): the
// with-object's inherited `has`/`get` traps must take priority over the
// local, per `with` dynamic-scoping semantics. ---
{
  const base = {
    has(_t: any, k: string) {
      return k === "lx";
    },
    get(_t: any, k: string) {
      return k === "lx" ? 333 : undefined;
    },
  };
  const handler: any = Object.create(base);
  function readLocal() {
    let lx = 1;
    with (new Proxy({}, handler) as any) {
      return lx;
    }
  }
  checks.push(readLocal() === 333);
}

// --- 4. Local variable write inside `with` (OpSetWithOrLocal): an
// inherited `has` trap must still be found so the with-object's `set`
// trap runs instead of silently falling back to the local. ---
{
  const log: string[] = [];
  const base = {
    has(_t: any, k: string) {
      return k === "lx";
    },
  };
  const handler: any = Object.create(base);
  handler.set = function (_t: any, k: string, v: any) {
    log.push("set:" + k + "=" + v);
    return true;
  };
  function writeLocal() {
    let lx = 1;
    with (new Proxy({}, handler) as any) {
      lx = 444;
    }
    return lx; // must be untouched: the with-object claimed the binding
  }
  checks.push(writeLocal() === 1);
  checks.push(log.indexOf("set:lx=444") !== -1);
}

// --- 5-6. Compound assignment inside `with` (OpResolveWithBinding then
// OpGetWithByBinding/OpSetWithByBinding), inherited get/has/set traps. ---
{
  const log: string[] = [];
  const base = {
    has(_t: any, k: string) {
      return k === "lx";
    },
    get(_t: any, k: string) {
      return k === "lx" ? 10 : undefined;
    },
    set(_t: any, k: string, v: any) {
      log.push("set:" + k + "=" + v);
      return true;
    },
  };
  const handler: any = Object.create(base);
  function compound() {
    let lx = 1;
    with (new Proxy({}, handler) as any) {
      lx += 5;
    }
    return lx;
  }
  checks.push(compound() === 1); // local untouched
  checks.push(log.indexOf("set:lx=15") !== -1); // 10 (from get) + 5
}

// --- 7. A TypeDictObject handler (a TS enum at runtime) must not panic
// on any `with` path (global read/write, local read/write, compound). ---
{
  enum DictHandler7 {
    A,
    B,
  }
  function useDictHandler() {
    let lx = 1;
    with (new Proxy({ wGlobal: 5, lx: 6 }, DictHandler7 as any) as any) {
      wGlobal;
      wGlobal = 9;
      lx;
      lx = 9;
      lx += 1;
    }
    return "no-panic";
  }
  checks.push(useDictHandler() === "no-panic");
}

checks.every((c) => c === true);

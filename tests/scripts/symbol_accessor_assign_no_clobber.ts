// expect: true
// no-typecheck
// setOwnCheckedByKey (and, for a string key on RegExp/Map/Set/Promise/
// BoundFunction/NativeFunction(WithProps), setOwnChecked - that call site
// never even pre-checked for an accessor the way TypeFunction/TypeClosure's
// callers in pkg/vm/op_setprop.go did) unconditionally wrote the assigned
// value through DefineOwnPropertyByKey/PlainObject.SetOwn whenever the
// target key already existed - including when it named an existing
// ACCESSOR. Ordinary [[Set]] (obj[key] = v) must instead call the
// accessor's setter if it has one, or - getter-only - silently no-op in
// sloppy mode / throw a TypeError in strict mode. It must never convert
// the accessor into a data property.
//
// Fixed by a new shared helper, setOwnAccessorSetter
// (pkg/vm/properties_table.go), that both setOwnChecked and
// setOwnCheckedByKey now consult first, mirroring the accessor-setter
// check pkg/vm/op_setprop.go's TypeObject (plain object) case already had.
//
// This file needs `// no-typecheck`: the TypeScript-checked default this
// repo's tests otherwise run under always compiles as strict mode (see
// pkg/compiler/compiler.go's "TypeScript mode - always strict"), which
// would make every assertion below throw instead of exercising the silent
// sloppy-mode no-op this bug is actually about. The assertions that want
// strict-mode throwing opt back into it locally via their own
// "use strict" directive.
//
// Getting the strict-mode assertions below to work at all required a
// second, causally-coupled fix: every OpSetIndex (bracket-notation
// assignment) dispatch site that calls opSetPropSymbol/opSetProp
// unconditionally did `if !ok { return status, res }` without checking
// vm.unwinding first - so a thrown exception (this fix's new strict-mode
// TypeError included) could never actually be caught by an enclosing
// try/catch; it just aborted the whole VM run. Fixed by mirroring
// OpSetProp's dot-notation dispatch (and the one already-correct
// TypeObject/callable branch of OpSetIndex itself), which check
// `vm.unwinding` and `goto reloadFrame` instead. See pkg/vm/vm.go's
// OpSetIndex case.

const checks = [];

function hasAccessorDesc(desc, hasSetter) {
  return (
    desc !== undefined &&
    typeof desc.get === "function" &&
    (hasSetter ? typeof desc.set === "function" : desc.set === undefined)
  );
}

// --- Function: assigning through an existing getter-only accessor must
// leave the backing state (and the accessor itself) untouched, and must
// not throw in sloppy mode. ---
(function () {
  let backing = 1;
  function fn() {}
  const sym = Symbol("k1");
  Object.defineProperty(fn, sym, {
    get() {
      return backing;
    },
    configurable: true,
  });
  fn[sym] = 2; // must not throw, must not touch backing, must not clobber
  checks.push(backing === 1);
  const desc = Object.getOwnPropertyDescriptor(fn, sym);
  checks.push(hasAccessorDesc(desc, false));
  checks.push(desc.get() === 1);
})();

// --- Function: assigning through an existing getter+setter accessor must
// call the setter (whatever it does), not write a data property. ---
(function () {
  let backing = 0;
  function fn() {}
  const sym = Symbol("k2");
  Object.defineProperty(fn, sym, {
    get() {
      return backing;
    },
    set(v) {
      backing = v * 10;
    },
    configurable: true,
  });
  fn[sym] = 3;
  checks.push(backing === 30);
  const desc = Object.getOwnPropertyDescriptor(fn, sym);
  checks.push(hasAccessorDesc(desc, true));
  checks.push(desc.get() === 30);
})();

// --- Map (an exotic kind whose symbol-set call site, unlike Function's,
// had no case at all before PR #333): getter-only accessor, sloppy mode. ---
(function () {
  let backing = "g";
  const m = new Map();
  const sym = Symbol("k3");
  Object.defineProperty(m, sym, {
    get() {
      return backing;
    },
    configurable: true,
  });
  m[sym] = "x";
  checks.push(backing === "g");
  checks.push(hasAccessorDesc(Object.getOwnPropertyDescriptor(m, sym), false));
})();

// --- Map: getter+setter accessor via a symbol key. ---
(function () {
  let backing = 0;
  const m = new Map();
  const sym = Symbol("k4");
  Object.defineProperty(m, sym, {
    get() {
      return backing;
    },
    set(v) {
      backing = v + 1;
    },
    configurable: true,
  });
  m[sym] = 5;
  checks.push(backing === 6);
  checks.push(hasAccessorDesc(Object.getOwnPropertyDescriptor(m, sym), true));
})();

// --- Map: the same getter+setter accessor via a STRING key - this call
// site (setOwnChecked, not setOwnCheckedByKey) never even had a precheck
// for an existing accessor, string key or symbol, so a setter was never
// invoked at all here (a distinct, arguably worse instance of the same
// underlying gap in the shared helper). ---
(function () {
  let backing = 0;
  const m = new Map();
  Object.defineProperty(m, "custom", {
    get() {
      return backing;
    },
    set(v) {
      backing = v + 100;
    },
    configurable: true,
  });
  m.custom = 7;
  checks.push(backing === 107);
  checks.push(hasAccessorDesc(Object.getOwnPropertyDescriptor(m, "custom"), true));
})();

// --- Strict mode, symbol key: assigning through a getter-only accessor
// must throw a TypeError instead of silently no-op'ing. Opts back into
// strict mode locally - the rest of this file is sloppy only because of
// the `// no-typecheck` directive above. ---
(function () {
  "use strict";
  let backing = 1;
  let threw = false;
  function fn() {}
  const sym = Symbol("k5");
  Object.defineProperty(fn, sym, {
    get() {
      return backing;
    },
    configurable: true,
  });
  try {
    fn[sym] = 2;
  } catch (e) {
    threw = e instanceof TypeError;
  }
  checks.push(threw);
  checks.push(backing === 1);
})();

// --- Strict mode, STRING key: same as above but through opSetProp's
// TypeFunction case rather than opSetPropSymbol. Before this fix, a
// getter-only accessor here silently no-op'd in *both* modes (PlainObject
// SetOwn saw the accessor field's writable:false and returned without
// ever consulting strict mode) - so this is a genuinely new throw path,
// not just the symbol-key one becoming stricter. ---
(function () {
  "use strict";
  let backing = 1;
  let threw = false;
  function fn() {}
  Object.defineProperty(fn, "ro", {
    get() {
      return backing;
    },
    configurable: true,
  });
  try {
    fn.ro = 2;
  } catch (e) {
    threw = e instanceof TypeError;
  }
  checks.push(threw);
  checks.push(backing === 1);
})();

// --- A throwing setter must propagate to an enclosing try/catch in the
// *same* frame - exercises setOwnAccessorSetter's own exception-wrapping
// path (the `vm.Call` error branch), which nothing above reaches since
// every setter there returns normally. ---
(function () {
  let caughtMessage = null;
  function fn() {}
  const sym = Symbol("k6");
  Object.defineProperty(fn, sym, {
    set() {
      throw new Error("boom");
    },
    configurable: true,
  });
  try {
    fn[sym] = 1;
  } catch (e) {
    caughtMessage = e.message;
  }
  checks.push(caughtMessage === "boom");
})();

// --- Same throwing setter, but raised from *inside* a native callback
// (Array.prototype.forEach) with the try/catch outside it. setOwnAccessorSetter
// re-enters the VM via vm.Call for every one of these nine exotic kinds -
// this crosses a native boundary on the way back out, the OpSetIndex
// unwinding fix's riskiest case (see this file's header comment). ---
(function () {
  let caughtMessage = null;
  function fn() {}
  const sym = Symbol("k7");
  Object.defineProperty(fn, sym, {
    set() {
      throw new Error("boom2");
    },
    configurable: true,
  });
  try {
    [1].forEach(function () {
      fn[sym] = 1;
    });
  } catch (e) {
    caughtMessage = e.message;
  }
  checks.push(caughtMessage === "boom2");
})();

checks.every((c) => c === true);

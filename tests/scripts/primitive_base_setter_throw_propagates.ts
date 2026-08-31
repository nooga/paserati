// expect: true
// no-typecheck

// A setter or Proxy set-trap reached through a PRIMITIVE base's prototype
// chain must propagate its exception, exactly like the identical shape on an
// object base already did.
//
// opSetProp's primitive path (pkg/vm/op_setprop.go) coerces the primitive to
// a temp object and walks the prototype chain looking for a setter or a Proxy
// set trap. Both call sites were written as `if _, err := vm.Call(...); err
// == nil { return success }` - so when the setter or trap *threw*, the error
// was silently discarded, the walk carried on up the chain instead of
// stopping at the setter that had already run, and execution fell through to
// the "no setter found - silently succeed" return at the bottom. The
// assignment reported success and the exception vanished: another instance of
// the false-success family tracked in paserati#65. Confirmed by instrumenting
// the walk: it logged finding the setter, then continued to the next link.
//
// Found while diffing #142's unwinding fix against real `main`: the swallowed
// error was what made language/types/reference/put-value-prop-base-primitive-realm.js
// look like it was merely asserting the wrong count.

let caughtSetter = false;
Object.defineProperty(Number.prototype, "boom1", {
  set: function () {
    throw new Error("boom-setter");
  },
  configurable: true,
});
let n = 0;
try {
  n.boom1 = 1;
} catch (e) {
  caughtSetter = e instanceof Error && e.message === "boom-setter";
}

let caughtTrap = false;
Object.setPrototypeOf(
  String.prototype,
  new Proxy(
    {},
    {
      set: function () {
        throw new Error("boom-trap");
      },
    }
  )
);
let s = "str";
try {
  s.boom2 = 1;
} catch (e) {
  caughtTrap = e instanceof Error && e.message === "boom-trap";
}

// A NON-throwing setter on a primitive base must still work, and the
// assignment expression must still evaluate to its RHS.
let setterRan = 0;
Object.defineProperty(Boolean.prototype, "ok", {
  set: function () {
    setterRan += 1;
  },
  configurable: true,
});
let b = true;
const assigned = (b.ok = 7);
const nonThrowingStillWorks = setterRan === 1 && assigned === 7;

// Control: the same throwing-setter shape on an ordinary object base, which
// always propagated correctly - guards against "fixed" by breaking both.
let caughtOnObject = false;
const obj: any = {};
Object.defineProperty(obj, "boom3", {
  set: function () {
    throw new Error("boom-obj");
  },
});
try {
  obj.boom3 = 1;
} catch (e) {
  caughtOnObject = e instanceof Error && e.message === "boom-obj";
}

caughtSetter && caughtTrap && nonThrowingStillWorks && caughtOnObject;

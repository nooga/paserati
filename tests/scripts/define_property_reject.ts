// Object.defineProperty must throw a TypeError when the redefinition is
// rejected, instead of silently returning the object unchanged (issue #126).
// The rejection rules are ValidateAndApplyPropertyDescriptor (ES2025 10.1.6.3),
// including the cases where a *non-configurable* property may still be
// redefined because nothing actually changes.
// expect: pass

const results: string[] = [];

function check(name: string, actual: any, expected: any) {
  if (actual !== expected) {
    results.push(name + ": got " + String(actual) + ", want " + String(expected));
  }
}

function threw(name: string, fn: () => void) {
  let caught = false;
  try {
    fn();
  } catch (e) {
    caught = e instanceof TypeError;
  }
  check(name, caught, true);
}

function didNotThrow(name: string, fn: () => void) {
  try {
    fn();
    check(name, true, true);
  } catch (e) {
    results.push(name + ": threw " + String(e));
  }
}

// --- rejections that used to pass silently ----------------------------------
const frozen: any = { a: 1 };
Object.freeze(frozen);
threw("redefine value on frozen", () => { Object.defineProperty(frozen, "a", { value: 9 }); });
check("value unchanged", frozen.a, 1);
threw("re-widen writable on frozen", () => { Object.defineProperty(frozen, "a", { writable: true }); });
threw("re-widen configurable on frozen", () => { Object.defineProperty(frozen, "a", { configurable: true }); });
threw("flip enumerable on frozen", () => { Object.defineProperty(frozen, "a", { enumerable: false }); });

const locked: any = {};
Object.defineProperty(locked, "b", { value: 1, configurable: false });
threw("redefine non-configurable value", () => { Object.defineProperty(locked, "b", { value: 2 }); });
threw("data to accessor when non-configurable", () => {
  Object.defineProperty(locked, "b", { get: () => 3 });
});

// Same on a callable, whose properties live in a side table.
const fn: any = function () {};
fn.a = 1;
Object.freeze(fn);
threw("redefine value on frozen fn", () => { Object.defineProperty(fn, "a", { value: 9 }); });
check("fn value unchanged", fn.a, 1);
threw("add to frozen fn via defineProperty", () => { Object.defineProperty(fn, "n", { value: 1 }); });

// --- redefinitions a non-configurable property must still allow -------------
// SameValue on the value is a no-op, not a change.
didNotThrow("restate same value", () => { Object.defineProperty(frozen, "a", { value: 1 }); });
// ... and SameValue is not ===: NaN matches NaN, +0 does not match -0.
const nan: any = {};
Object.defineProperty(nan, "n", { value: NaN });
didNotThrow("restate NaN", () => { Object.defineProperty(nan, "n", { value: NaN }); });
const zero: any = {};
Object.defineProperty(zero, "z", { value: 0 });
threw("negative zero is a change", () => { Object.defineProperty(zero, "z", { value: -0 }); });

// An empty descriptor never rejects an existing property.
didNotThrow("empty descriptor on frozen", () => { Object.defineProperty(frozen, "a", {}); });

// A non-configurable accessor accepts a redefinition that changes nothing:
// filling in an absent setter with undefined, or restating the same getter.
const acc: any = {};
const getFunc = () => 0;
Object.defineProperty(acc, "g", { get: getFunc });
didNotThrow("set undefined on getter-only", () => { Object.defineProperty(acc, "g", { set: undefined }); });
didNotThrow("restate same getter", () => { Object.defineProperty(acc, "g", { get: getFunc }); });
didNotThrow("empty descriptor on accessor", () => { Object.defineProperty(acc, "g", {}); });
check("getter survived", acc.g, 0);
threw("different getter is a change", () => { Object.defineProperty(acc, "g", { get: () => 1 }); });
threw("accessor to data when non-configurable", () => { Object.defineProperty(acc, "g", { value: 5 }); });

// --- a callable's synthesized properties keep their real attributes ---------
// "length" is configurable, so defining over it twice must work - it used to be
// materialized with all-false attributes, making the second define a rejection.
function target(a: any) { return a; }
didNotThrow("define length once", () => { Object.defineProperty(target, "length", { value: undefined }); });
didNotThrow("define length twice", () => { Object.defineProperty(target, "length", { value: null }); });

// --- Reflect.defineProperty reports the same rejection as false -------------
check("Reflect on frozen", Reflect.defineProperty(frozen, "a", { value: 9 }), false);
check("Reflect no-op on frozen", Reflect.defineProperty(frozen, "a", { value: 1 }), true);
check("Reflect on fresh", Reflect.defineProperty({}, "x", { value: 1 }), true);

results.length === 0 ? "pass" : results.join("; ");

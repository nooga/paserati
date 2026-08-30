// Object.freeze / seal / preventExtensions / isExtensible must work on values
// whose own properties live in a side table rather than on the value itself -
// functions of every flavour, RegExps, Maps and Sets (issue #123).
//
// NB: test scripts run in strict mode, so a rejected write throws here where
// sloppy code would silently do nothing.
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

// --- preventExtensions on a callable ---------------------------------------
const pe: any = function () {};
check("isExtensible before", Object.isExtensible(pe), true);
Object.preventExtensions(pe);
check("isExtensible after", Object.isExtensible(pe), false);
threw("add after preventExtensions", () => { pe.bar = 2; });
check("bar not added", pe.bar, undefined);
threw("defineProperty after preventExtensions", () => {
  Object.defineProperty(pe, "z", { value: 1 });
});

// preventExtensions leaves existing properties alone.
const peExisting: any = function () {};
peExisting.a = 1;
Object.preventExtensions(peExisting);
peExisting.a = 5;
check("existing write still ok", peExisting.a, 5);
check("existing still configurable", delete peExisting.a, true);

// --- freeze / seal on a callable with no properties yet ---------------------
const empty: any = function () {};
Object.freeze(empty);
check("isFrozen(empty fn)", Object.isFrozen(empty), true);
check("isExtensible(frozen fn)", Object.isExtensible(empty), false);
threw("add to frozen fn", () => { empty.x = 1; });
check("x not added", empty.x, undefined);

const sealed: any = function () {};
Object.seal(sealed);
check("isSealed(empty fn)", Object.isSealed(sealed), true);
check("isFrozen(sealed fn)", Object.isFrozen(sealed), false);

// A frozen function is still a function.
const callable: any = function () { return 7; };
callable.tag = "t";
Object.freeze(callable);
check("frozen fn still calls", callable(), 7);
check("frozen fn keeps value", callable.tag, "t");
threw("write to frozen fn property", () => { callable.tag = "u"; });
check("frozen fn value unchanged", callable.tag, "t");

// --- the synthesized own properties are covered too -------------------------
function named(a: any, b: any) { return a + b; }
Object.freeze(named);
const nameDesc: any = Object.getOwnPropertyDescriptor(named, "name");
check("frozen name value", nameDesc.value, "named");
check("frozen name writable", nameDesc.writable, false);
check("frozen name configurable", nameDesc.configurable, false);
const lenDesc: any = Object.getOwnPropertyDescriptor(named, "length");
check("frozen length value", lenDesc.value, 2);
check("frozen length configurable", lenDesc.configurable, false);
const protoDesc: any = Object.getOwnPropertyDescriptor(named, "prototype");
check("frozen prototype writable", protoDesc.writable, false);
threw("write frozen prototype", () => { (named as any).prototype = {}; });

// Sealing keeps them writable-as-they-were but non-configurable.
function sealedFn(a: any) { return a; }
Object.seal(sealedFn);
check("sealed name configurable", (Object.getOwnPropertyDescriptor(sealedFn, "name") as any).configurable, false);
check("sealed prototype writable", (Object.getOwnPropertyDescriptor(sealedFn, "prototype") as any).writable, true);

// preventExtensions must NOT touch attributes.
function peFn() {}
Object.preventExtensions(peFn);
check("pe name configurable", (Object.getOwnPropertyDescriptor(peFn, "name") as any).configurable, true);
check("pe prototype writable", (Object.getOwnPropertyDescriptor(peFn, "prototype") as any).writable, true);

// --- every callable flavour -------------------------------------------------
const arrow: any = () => 1;
Object.freeze(arrow);
check("isFrozen(arrow)", Object.isFrozen(arrow), true);

class Klass {}
Object.freeze(Klass);
check("isFrozen(class)", Object.isFrozen(Klass), true);

const bound: any = function () {}.bind(null);
Object.freeze(bound);
check("isFrozen(bound)", Object.isFrozen(bound), true);

Object.preventExtensions(Math.max);
check("isExtensible(native)", Object.isExtensible(Math.max), false);

// --- RegExp, Map and Set ----------------------------------------------------
const re: any = /x/;
Object.preventExtensions(re);
check("isExtensible(regexp)", Object.isExtensible(re), false);
threw("add to non-extensible regexp", () => { re.q = 1; });

const frozenRe: any = /y/;
Object.freeze(frozenRe);
check("isFrozen(regexp)", Object.isFrozen(frozenRe), true);
check("regexp still matches", frozenRe.test("y"), true);

const map: any = new Map();
Object.preventExtensions(map);
threw("add to non-extensible map", () => { map.q = 1; });
map.set("k", 1);
check("frozen-ish map still works", map.get("k"), 1);

const set: any = new Set();
Object.freeze(set);
check("isFrozen(set)", Object.isFrozen(set), true);

// Computed member assignment goes through a different opcode than `f.x = v`,
// and must reject the same way.
const indexed: any = function () {};
indexed["a"] = 1;
Object.freeze(indexed);
threw("computed write to frozen fn", () => { indexed["a"] = 2; });
threw("computed add to frozen fn", () => { indexed["b"] = 1; });
check("computed value unchanged", indexed["a"], 1);

// --- a fresh callable is extensible whichever opcode built its table ---------
const key = "m";
class Computed { static [key]() { return 1; } }
check("isExtensible(computed static)", Object.isExtensible(Computed), true);
(Computed as any).z = 1;
check("computed static add", (Computed as any).z, 1);

results.length === 0 ? "pass" : results.join("; ");

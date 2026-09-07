// expect: true
// Regression test for #297: Reflect.deleteProperty threw "called on
// non-object" for any callable target, though functions and classes are
// ordinary objects. Also covers symbol keys on plain objects (previously
// stringified) and the non-configurable -> false (not throw) rule.
const results: boolean[] = [];

// Plain function target
function F() {}
(F as any).x = 1;
results.push(Reflect.deleteProperty(F, "x") === true && (F as any).x === undefined);

// Class target with a static field (undici's pattern)
class C {
  static y = 2;
}
results.push(Reflect.deleteProperty(C, "y") === true && (C as any).y === undefined);

// Arrow function and native function targets
const arrow = () => 1;
(arrow as any).z = 5;
results.push(Reflect.deleteProperty(arrow, "z") === true && (arrow as any).z === undefined);
results.push(Reflect.deleteProperty(Math.max, "nope") === true);

// Bound function target
const bound = F.bind(null);
(bound as any).b = 3;
results.push(Reflect.deleteProperty(bound, "b") === true && (bound as any).b === undefined);

// Non-configurable own property on a function: reports false, does not throw
Object.defineProperty(F, "fixed", { value: 1, configurable: false });
results.push(Reflect.deleteProperty(F, "fixed") === false && (F as any).fixed === 1);

// "prototype" of an ordinary function is non-configurable
results.push(Reflect.deleteProperty(F, "prototype") === false && F.prototype !== undefined);

// Intrinsic name/length are configurable and deletable
function G() {}
results.push(Reflect.deleteProperty(G, "name") === true && !Object.prototype.hasOwnProperty.call(G, "name"));

// Symbol key on a plain object
const s = Symbol("k");
const o: any = { [s]: 42, a: 1 };
results.push(Reflect.deleteProperty(o, s) === true && !Object.prototype.hasOwnProperty.call(o, s) && o.a === 1);

// Array element and length
const arr = [1, 2, 3];
results.push(Reflect.deleteProperty(arr, "1") === true && !Object.prototype.hasOwnProperty.call(arr, 1) && arr.length === 3);
results.push(Reflect.deleteProperty(arr, "length") === false);

// Map with an expando property
const m: any = new Map();
m.tag = "t";
results.push(Reflect.deleteProperty(m, "tag") === true && m.tag === undefined);

// Proxy: trap result is reported, invariant violation throws
const p1 = new Proxy({ q: 1 }, { deleteProperty: () => false });
results.push(Reflect.deleteProperty(p1, "q") === false);
const frozenTarget = Object.freeze({ q: 1 });
let threw = false;
try {
  Reflect.deleteProperty(new Proxy(frozenTarget, { deleteProperty: () => true }), "q");
} catch (e) {
  threw = e instanceof TypeError;
}
results.push(threw);

// Non-object targets still throw
let nonObjThrew = false;
try {
  Reflect.deleteProperty(1 as any, "p");
} catch (e) {
  nonObjThrew = e instanceof TypeError;
}
results.push(nonObjThrew);

results.every((r) => r) && results.length === 15;

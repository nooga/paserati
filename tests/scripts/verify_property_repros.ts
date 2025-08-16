// expect: true
// Minimal engine-level repros (no harness dependencies)

// 1) restore accessor on symbol-keyed property
var s = Symbol("x");
var obj = {};
var desc = { enumerable: true, configurable: true } as any;
(desc as any).get = function () {
  return 42;
};
(desc as any).set = function (_v: any) {};
Object.defineProperty(obj, s, desc);
var ok1 = true;
var d1 = Object.getOwnPropertyDescriptor(obj, s);
ok1 =
  ok1 &&
  d1 !== undefined &&
  d1.get === (desc as any).get &&
  d1.set === (desc as any).set &&
  d1.enumerable === true &&
  d1.configurable === true;
var original = d1 as any;
Object.defineProperty(obj, s, desc);
Object.defineProperty(obj, s, original);
var d2 = Object.getOwnPropertyDescriptor(obj, s);
ok1 = ok1 && Object.prototype.hasOwnProperty.call(obj, s) === true;
ok1 = ok1 && (obj as any)[s] === 42;
ok1 =
  ok1 &&
  d2 !== undefined &&
  (d2 as any).get === (desc as any).get &&
  (d2 as any).set === (desc as any).set;

// 2) getOwnPropertyDescriptor for undefined data and accessor properties
var sample: any = { bar: undefined };
Object.defineProperty(sample, "baz", {
  get: function () {},
  configurable: true,
  enumerable: true,
});
var dBar = Object.getOwnPropertyDescriptor(sample, "bar");
var dBaz = Object.getOwnPropertyDescriptor(sample, "baz");
var ok2 =
  dBar !== undefined &&
  dBaz !== undefined &&
  typeof (dBaz as any).get === "function";

ok1 && ok2;

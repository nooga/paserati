// Function() / AsyncFunction() / GeneratorFunction() return real functions:
// named "anonymous", with the right [[Prototype]], own properties and
// constructor (#114).
// no-typecheck
// expect: anonymous,true,true,length;name;prototype,3|AsyncFunction,true,TypeError|GeneratorFunction,true,1;2|1,4
const f = new Function("a", "b", "return a + b");
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const af = AsyncFunction("return 1");
let asyncNew = "no throw";
try { new af(); } catch (e) { asyncNew = e.name; }
const GeneratorFunction = Object.getPrototypeOf(function* () {}).constructor;
const gf = GeneratorFunction("yield 1; yield 2");
const MyU8 = new Function("return class MyU8 extends Uint8Array {}")();
[
  [f.name, Object.getPrototypeOf(f) === Function.prototype, f instanceof Function, Object.getOwnPropertyNames(f).join(";"), f(1, 2)].join(","),
  [af.constructor.name, Object.getPrototypeOf(af) === AsyncFunction.prototype, asyncNew].join(","),
  [gf.constructor.name, Object.getPrototypeOf(gf) === GeneratorFunction.prototype, [...gf()].join(";")].join(","),
  [MyU8.BYTES_PER_ELEMENT, new MyU8(4).length].join(","),
].join("|");

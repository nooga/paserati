// expect: ["GeneratorFunction","AsyncGeneratorFunction","AsyncFunction",false,"function* anonymous(a\n) {\nyield a; yield a * 2;\n}","3,6",true,true,"false",1,"async function* anonymous(x\n) {\nyield x\n}",true,"function"]
// skip-typecheck
// paserati#524: %GeneratorFunction% and %AsyncGeneratorFunction% didn't exist -
// Object.getPrototypeOf(function*(){}).constructor fell through to Function,
// so it built ordinary functions. They create generator functions now, with
// the CreateDynamicFunction source text.
const GF = Object.getPrototypeOf(function* () {}).constructor;
const AGF = Object.getPrototypeOf(async function* () {}).constructor;
const AF = Object.getPrototypeOf(async function () {}).constructor;
const g = new GF("a", "yield a; yield a * 2;");
const out = [GF.name, AGF.name, AF.name, GF === Function, String(g), [...g(3)].join(), Object.getPrototypeOf(g) === GF.prototype,
  GF.prototype.constructor === GF, JSON.stringify(Object.getOwnPropertyDescriptor(GF.prototype, "constructor").writable), GF.length,
  String(new AGF("x", "yield x")), Object.getPrototypeOf(AGF("yield 1")) === AGF.prototype, typeof GF("yield 1")().next];
JSON.stringify(out);

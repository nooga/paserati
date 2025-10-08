// expect: baz
class C {
  1n() { return "baz"; }
}
let c = new C();
c["1"]();

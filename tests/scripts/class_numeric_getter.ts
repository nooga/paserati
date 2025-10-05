// expect: 0.1
class C {
  get .1() { return 0.1; }
}
const c = new C();
c['0.1'];

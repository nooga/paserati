// expect: 3
class Fault { n: number = 3; times(n: number): Fault { return this; } }
class Test {
  use<St>(st: St): Fault { return new Fault(); }
  mk<St>(st: St): Test { return this; }
}
function take(t: Test): void {}
const t = new Test();
take(t);
t.use(1).times(2).times(3).n;

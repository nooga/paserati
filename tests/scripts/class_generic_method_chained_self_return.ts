// expect: 7
class Test {
  n: number = 7;
  mk<St>(st: St): Test { return this; }
}
const t = new Test();
t.mk(1).mk("x").mk(true).n;

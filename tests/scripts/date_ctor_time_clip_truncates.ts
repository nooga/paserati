// expect: 1,-1,1790109925145,true,NaN,5,3
// skip-typecheck
// paserati#519: new Date(value) runs TimeClip, which applies
// ToIntegerOrInfinity - the fraction is dropped and -0 becomes +0. The
// constructor also skipped the 8.64e15 range check entirely.
const d = new Date(0);
d.setTime(5.9);
[
  new Date(1.7).getTime(),
  new Date(-1.7).getTime(),
  new Date(1790109925145.0535).getTime(),
  Object.is(new Date(-0.5).getTime(), 0),
  new Date(9e15).getTime(),
  d.getTime(),
  new Date(new Date(3.3)).getTime(),
].join();

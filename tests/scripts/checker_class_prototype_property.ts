// expect: 2,3,true
// A class constructor's `prototype` is an instance of the class.
class K {
  m(): number {
    return 1;
  }
}
class D extends K {
  extra(): number {
    return 3;
  }
}
abstract class A {
  abstract f(): number;
}
const p: K = K.prototype;
K.prototype.m = (): number => 2;
const dp: D = D.prototype;
const ap: A = A.prototype;
[K.prototype.m(), dp.extra(), p instanceof K || p === K.prototype].join(",");

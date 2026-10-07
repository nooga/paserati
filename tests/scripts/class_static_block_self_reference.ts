// expect: 3,A,5
// #608: static blocks and static field initializers see the class by name
class A {
  static t: number = 0;
  static n: string = A.name;
  static {
    A.t = 3;
  }
}
function local(): number {
  class B {
    static v: number = 0;
    static {
      B.v = 5;
    }
  }
  return B.v;
}
[A.t, A.n, local()].join(",");

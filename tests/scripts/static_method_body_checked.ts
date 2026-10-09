// expect: 3
class Base { static tag(): number { return 1; } }
class C extends Base {
  static n = 2;
  static f(): number { return super.tag() + this.n; }
}
C.f();

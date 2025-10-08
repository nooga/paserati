// expect: 42
class C {
  static x = 0;
  static {
    this.x = 42;
  }
}
C.x;

// expect: 5
class Outer {
  inner: any = class { f() { return 5; } };
  result: number;
  constructor() {
    class Derived extends this.inner {}
    this.result = (new Derived() as any).f();
  }
}
new Outer().result;

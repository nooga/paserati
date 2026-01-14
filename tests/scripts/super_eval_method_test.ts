// expect: 42
// Test: super method access in eval from class method (now supported!)

class A {
  getValue() { return 42; }
}

class B extends A {
  getX() {
    return eval('super.getValue()');
  }
}

const b = new B();
b.getX()

// Test class method accessing outer variable - basic case
// expect: 42
// no-typecheck

let x = 42;

class Test {
  method() {
    return x;
  }
}

let t = new Test();
t.method();

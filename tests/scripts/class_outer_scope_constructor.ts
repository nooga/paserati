// Test class constructor accessing outer variable
// expect: 42
// no-typecheck

let x = 42;

class Test {
  value: number;
  constructor() {
    this.value = x;
  }
}

let t = new Test();
t.value;

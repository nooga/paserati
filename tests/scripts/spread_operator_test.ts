// Test spread operator with super call
// expect: Test completed
class TestParent {
  static source = [3, 4, 5];
  constructor(a: number, b: number, c: number, d: number, e: number) {
    console.log("Parent constructor called with:", a, b, c, d, e);
  }
}

class TestChild extends TestParent {
  constructor() {
    super(1, 2, ...TestParent.source);
  }
}

const instance = new TestChild();
"Test completed";

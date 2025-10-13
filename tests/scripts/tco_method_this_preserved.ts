// Method tail call preserves 'this' context
// expect: 42
class MyClass {
  value: number;

  constructor() {
    this.value = 42;
  }

  countdown(n: number): number {
    if (n === 0) return this.value;
    return this.countdown(n - 1);
  }
}

const obj = new MyClass();
obj.countdown(10);

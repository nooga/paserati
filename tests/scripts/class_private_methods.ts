// expect: all private method tests passed
// Test private instance methods

class Calculator {
  #privateValue = 10;

  // Private instance method
  #multiply(x: number): number {
    return this.#privateValue * x;
  }

  // Public method that uses private method
  calculate(x: number): number {
    return this.#multiply(x);
  }

  test(): string {
    if (this.calculate(5) !== 50) {
      return "FAIL: Expected 50";
    }
    return "all private method tests passed";
  }
}

// Test: Call private method through public method
const calc = new Calculator();
calc.test();

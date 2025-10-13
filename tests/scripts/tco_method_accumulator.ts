// Method tail call with accumulator
// expect: 5050
class Calculator {
  sum(n: number, acc: number): number {
    if (n === 0) return acc;
    return this.sum(n - 1, acc + n);
  }
}

const calc = new Calculator();
calc.sum(100, 0);

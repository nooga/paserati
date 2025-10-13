// Simple method tail call
// expect: 0
class Counter {
  countdown(n: number): number {
    if (n === 0) return 0;
    return this.countdown(n - 1);
  }
}

const counter = new Counter();
counter.countdown(100);

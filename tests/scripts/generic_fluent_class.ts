// Generic class with fluent pattern
// expect: Result: processed-hello-42

class Pipeline<T> {
  private value: T;

  constructor(initial: T) {
    this.value = initial;
  }

  map<U>(fn: (x: T) => U): Pipeline<U> {
    return new Pipeline<U>(fn(this.value));
  }

  tap(fn: (x: T) => void): Pipeline<T> {
    fn(this.value);
    return this;
  }

  getValue(): T {
    return this.value;
  }
}

// Chain pipeline operations
const result = new Pipeline<string>("hello")
  .map((s: string) => s + "-42")
  .map((s: string) => "processed-" + s)
  .getValue();

"Result: " + result;

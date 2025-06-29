// Test for generic method chaining issue

class Box<T> {
  private value: T;

  constructor(val: T) {
    this.value = val;
  }

  map<U>(fn: (x: T) => U): Box<U> {
    return new Box(fn(this.value));
  }

  getValue(): T {
    return this.value;
  }
}

// Test 1: Basic chaining
const box1 = new Box(42);
const box2 = box1.map(x => x * 2);  // Box<number>
const box3 = box2.map(x => "value: " + x);  // Box<string>

// This should work but likely fails:
const result = box3.getValue();  // Should be string
console.log("Result:", result);

// Test 2: Multiple chains
const chain = new Box(10)
  .map(x => x + 5)      // Box<number>
  .map(x => x * 2)      // Box<number>
  .map(x => x > 20);    // Box<boolean>

// This should work but likely fails:
const chainResult = chain.getValue();  // Should be boolean
console.log("Chain result:", chainResult);

// expect: Result: value: 84
// expect: Chain result: true
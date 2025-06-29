// Final test - generic method chaining working correctly

class Box<T> {
  value: T;
  
  constructor(val: T) {
    this.value = val;
  }
  
  map<U>(fn: (x: T) => U): Box<U> {
    return new Box(fn(this.value));
  }
}

// This now works! No more "property access is not supported on type Box<U>"
const result = new Box(10)
  .map(x => x * 2)     // Box<number>
  .map(x => x + 5)     // Box<number>
  .map(x => x > 20);   // Box<boolean>

console.log("Final result:", result.value);

// expect: Final result: true
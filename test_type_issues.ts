// Test case 1: Class hoisting
class TestClass<T> {
  constructor(public data: T) {}
  
  transform<U>(fn: (x: T) => U): TestClass<U> {
    return new TestClass(fn(this.data));
  }
}

const test1 = new TestClass("hello");

// Test case 2: Type narrowing  
function testNarrowing(input: unknown): string {
  if (typeof input !== "string") {
    throw new Error("Not a string");
  }
  return input.toUpperCase(); // Should work after narrowing
}

// Test case 3: Built-in globals
const num = parseInt("42");

// Test case 4: Generic method chaining
const test2 = test1.transform(s => s.length);
const test3 = test2.transform(n => n * 2);
// Test case 1: Basic class hoisting
class TestClass {
  data: string;
  constructor(data: string) {
    this.data = data;
  }
}

const test1 = new TestClass("hello");

// Test case 2: Type narrowing  
function testNarrowing(input: unknown): string {
  if (typeof input !== "string") {
    throw new Error("Not a string");
  }
  return input; // Should work after narrowing
}

// Test case 3: Built-in globals
const num = parseInt("42");

console.log(test1.data);
console.log(testNarrowing("hello"));
console.log(num);
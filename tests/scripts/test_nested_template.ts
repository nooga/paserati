// Test template literal with generic type variables

class Container<T> {
  data: T;
  
  constructor(data: T) {
    this.data = data;
  }
  
  format(): string {
    return `Container holds: ${this.data}`;
  }
}

const strContainer = new Container("hello");
const numContainer = new Container(42);

// Both should work with template literals
strContainer.format();
numContainer.format();

"Test completed";

// expect: Test completed
// Test generic constructor type inference

class Container<T> {
  data: T;
  
  constructor(data: T) {
    this.data = data;
  }
  
  // Explicit type parameter - should work
  copyExplicit(): Container<T> {
    return new Container<T>(this.data);
  }
  
  // Type inference - may not work yet
  copyInferred(): Container<T> {
    return new Container(this.data);
  }
}

const container = new Container("hello");
const copy1 = container.copyExplicit();
const copy2 = container.copyInferred(); // This might fail

console.log("Test completed");
// Simple test for generic constructor inference

class Container<T> {
  private value: T;

  constructor(initialValue: T) {
    this.value = initialValue;
  }

  transform<U>(mapper: (value: T) => U): Container<U> {
    const newValue = mapper(this.value);
    return new Container(newValue); // This should work with type inference
  }
}

const numContainer = new Container(42);
const stringContainer = numContainer.transform(x => "value: " + x);

console.log("Test completed successfully");
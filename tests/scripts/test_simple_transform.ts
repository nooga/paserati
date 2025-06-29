// Simple test of transform with concrete return type

class Container<T> {
  data: T;
  
  constructor(data: T) {
    this.data = data;
  }

  transform<U>(fn: (item: T) => U): Container<U> {
    const result = fn(this.data);
    return new Container(result);
  }
}

const container = new Container(42);

// This lambda should have return type 'string', not 'any'
const transformed = container.transform((num) => {
  return "value: " + num;
});

"Simple transform test completed";

// expect: Simple transform test completed
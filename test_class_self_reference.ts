// Test class self-reference scenarios

// Simple non-generic class
class SimpleClass {
  value: number;
  
  constructor(value: number) {
    this.value = value;
  }
  
  copy(): SimpleClass {
    return new SimpleClass(this.value); // Should work
  }
}

// Generic class with explicit type parameter
class GenericClass<T> {
  data: T;
  
  constructor(data: T) {
    this.data = data;
  }
  
  copyExplicit(): GenericClass<T> {
    return new GenericClass<T>(this.data); // Should work
  }
  
  copyInferred(): GenericClass<T> {
    return new GenericClass(this.data); // Should work with type inference
  }
}

const simple = new SimpleClass(42);
const copy1 = simple.copy();

const generic = new GenericClass("hello");
const copy2 = generic.copyExplicit();
const copy3 = generic.copyInferred();

console.log("Test completed");
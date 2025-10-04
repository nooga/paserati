// Test ECMAScript private field (#) type checking

class Example {
  #secret: number = 42;

  getSecret(): number {
    return this.#secret; // Should be allowed
  }
}

const obj = new Example();
console.log(obj.#secret);  // Should error: private field accessed outside class

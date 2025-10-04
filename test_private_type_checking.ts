// Test type checker with private fields

class Example {
  // ECMAScript private field
  #secret: number = 42;

  // TypeScript private keyword
  private tsPrivate: string = "ts";

  // Public field for comparison
  public publicField: number = 100;

  getSecret(): number {
    return this.#secret; // Should be allowed
  }

  getTsPrivate(): string {
    return this.tsPrivate; // Should be allowed
  }
}

const obj = new Example();
console.log(obj.publicField);  // Should work
console.log(obj.getSecret());   // Should work
console.log(obj.getTsPrivate()); // Should work

// These should fail type checking:
console.log(obj.#secret);       // Error: private field accessed outside class
console.log(obj.tsPrivate);     // Error: private property accessed outside class

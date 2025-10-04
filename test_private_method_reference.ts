class Test {
  #privateMethod() { return 42; }
  
  getMethod() {
    return this.#privateMethod;  // Should return the function itself
  }
  
  callMethod() {
    return this.#privateMethod();  // Should call and return 42
  }
}

const t = new Test();
console.log("Call works:", t.callMethod());  // Should print: Call works: 42
const fn = t.getMethod();  // Should get the function
console.log("Reference works:", fn());  // Should print: Reference works: 42

class TestClass {
  #privateField = 42;
  
  #privateMethod() {
    return this.#privateField;
  }
  
  #privateGetter() {
    return "getter";
  }
  
  #privateSetter(value: number) {
    this.#privateField = value;
  }
  
  publicMethod() {
    return this.#privateMethod();
  }
}
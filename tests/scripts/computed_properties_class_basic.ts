// Test basic computed properties in class bodies
const methodName = "dynamicMethod";

class TestClass {
  // Computed property declaration
  [methodName]: string = "computed value";
  
  // Computed method
  [methodName + "2"]() {
    return "computed method result";
  }
}

const instance = new TestClass();
console.log(instance.dynamicMethod);

instance.dynamicMethod;

// expect: computed value
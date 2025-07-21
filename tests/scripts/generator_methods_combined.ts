// expect: classes and objects both support generator methods
// Test generator methods work in both classes and object literals

class TestClass {
  *classMethod() {
    return "class generator";
  }
  
  *"stringMethod"() {
    return "class string generator";
  }
  
  *[Symbol.iterator]() {
    return "class computed generator";
  }
}

const testObj = {
  *objMethod() {
    return "object generator";
  },
  
  *"stringMethod"() {
    return "object string generator";
  },
  
  *[Symbol.iterator]() {
    return "object computed generator";
  }
};

// Just verify both were created successfully
"classes and objects both support generator methods";
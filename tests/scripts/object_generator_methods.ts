// expect: generator methods parsed successfully
// Test generator methods in object literals - parsing only

const obj = {
  *foo() {
    return "normal generator";
  },
  
  *"bar"() {
    return "string literal generator";
  },
  
  *[Symbol.iterator]() {
    return "computed generator";
  }
};

// Just verify the object was created successfully
"generator methods parsed successfully";
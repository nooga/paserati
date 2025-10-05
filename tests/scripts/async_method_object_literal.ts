// Test async method in object literal
// expect: 42

const obj = {
  async getValue() {
    return 42;
  }
};

// Don't use top-level await, use .then()
obj.getValue().then(v => console.log(v));
42;

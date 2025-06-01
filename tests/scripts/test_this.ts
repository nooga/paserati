// expect: 42
// Test 'this' keyword type checking

let obj = {
  value: 42,
  getValue: function () {
    return this.value;
  },
};

obj.getValue();

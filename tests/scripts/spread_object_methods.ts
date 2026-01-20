// Test spread with object methods
let obj = {
  value: 42,
  getValue: function() { return this.value; }
};
let extended = {...obj, newProp: "test"};
extended;
// expect: {value: 42, getValue: [Function (anonymous)], newProp: "test"}
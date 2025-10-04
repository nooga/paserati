// Test 'in' operator with function objects (constructors)
// expect: MAX_VALUE in Number: true, prototype in Array: true

const test1 = "MAX_VALUE" in Number;
const test2 = "prototype" in Array;

`MAX_VALUE in Number: ${test1}, prototype in Array: ${test2}`;

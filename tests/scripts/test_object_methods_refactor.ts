// Test that Object prototype methods work with the new system

let obj = { x: 42 };

// Test toString
console.log(obj.toString()); // expect: [object Object]

// Test hasOwnProperty
console.log(obj.hasOwnProperty("x")); // expect: true
console.log(obj.hasOwnProperty("y")); // expect: false

// Test valueOf
console.log(obj.valueOf() === obj); // expect: true

// Test Function prototype methods
function fn() { return 42; }
console.log(typeof fn.call); // expect: function
console.log(typeof fn.apply); // expect: function
console.log(typeof fn.bind); // expect: function

// Test that methods are callable
let result = fn.call(null);
console.log(result); // expect: 42
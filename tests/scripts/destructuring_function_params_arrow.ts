// Test arrow function parameter destructuring
let multiply = ([x, y]: [number, number]) => x * y;

let nums: [number, number] = [6, 7];
let product = multiply(nums);
product;
// expect: 42
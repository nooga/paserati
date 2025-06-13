// Test Function.prototype.call with native functions
let str = "hello world";

// Array.prototype.push is a native function
let arr: number[] = [1, 2, 3];

// Test basic functionality first
console.log("Array before:", arr);
arr.push(4);
console.log("Array after push:", arr);

// Test other native methods
console.log("String length:", str.length);
console.log("Array length:", arr.length);
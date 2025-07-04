// Test constructor property on primitive prototypes
console.log("Testing constructor property on primitive prototypes:");

// Test Number.prototype.constructor
console.log("Number.prototype.constructor:", Number.prototype.constructor);
console.log("Number.prototype.constructor === Number:", Number.prototype.constructor === Number);

// Test String.prototype.constructor
console.log("String.prototype.constructor:", String.prototype.constructor);
console.log("String.prototype.constructor === String:", String.prototype.constructor === String);

// Test on primitive values
let num = 42;
console.log("num.constructor:", num.constructor);
console.log("num.constructor === Number:", num.constructor === Number);

let str = "hello";
console.log("str.constructor:", str.constructor);
console.log("str.constructor === String:", str.constructor === String);

// Test Array.prototype.constructor
console.log("Array.prototype.constructor:", Array.prototype.constructor);
console.log("Array.prototype.constructor === Array:", Array.prototype.constructor === Array);

let arr = [1, 2, 3];
console.log("arr.constructor:", arr.constructor);
console.log("arr.constructor === Array:", arr.constructor === Array);
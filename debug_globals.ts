// Simple test to see builtin globals count
console.log("Testing builtin globals count");

// Test a few known builtins
console.log("Array:", typeof Array);
console.log("Object:", typeof Object);
console.log("String:", typeof String);
console.log("Number:", typeof Number);
console.log("Math:", typeof Math);

// Add a user global
let myGlobal = "test";
console.log("myGlobal:", myGlobal);
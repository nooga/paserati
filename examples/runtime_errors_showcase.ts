// ⚡ Runtime Error Showcase
// This file demonstrates various runtime execution errors

console.log("Testing runtime errors...");

// Reference error - accessing undefined variable
console.log(undefinedVariable);  // Should cause reference error

// Property access on undefined/null
let obj: any = null;
console.log(obj.someProperty);  // Accessing property on null

// Invalid function call
let notAFunction: any = 42;
notAFunction();  // Trying to call a number as function

// Array index out of bounds behavior
let arr = [1, 2, 3];
console.log(arr[10]);  // Accessing non-existent index

// Division by zero
let result = 10 / 0;
console.log(result);  // Infinity in JavaScript, but might be different in Paserati

// Type error in object property access
let wrongType: any = "string";
console.log(wrongType.nonExistentMethod());  // Method doesn't exist on string

// Stack overflow (if we implement recursion limits)
function recursive(): never {
    return recursive();  // Infinite recursion
}
// recursive();  // Commented out to prevent hanging

console.log("If you see this, runtime errors were handled gracefully!");

// expect_runtime_error: Various runtime execution errors
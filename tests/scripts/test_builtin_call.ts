// Test with built-in functions
let arr = [1, 2, 3];
console.log("Array length:", arr.length);

// Test simple call without .call
function simple(x: number): number {
    return x * 2;
}

console.log("Simple call:", simple(5));

// Test .call on a builtin that should work
// For now just test basic functionality
console.log("Testing console.log directly");
console.log("Direct console.log works");
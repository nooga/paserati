function add(x: number, y: number): number {
    return x + y;
}

function multiply(x: number, y: number): number {
    return x * y;
}

// Test various function calls
console.log("Direct calls:");
console.log("add(3, 4):", add(3, 4));
console.log("multiply(3, 4):", multiply(3, 4));

console.log("\nFunction.prototype.call:");
console.log("add.call(null, 3, 4):", add.call(null, 3, 4));
console.log("multiply.call(null, 3, 4):", multiply.call(null, 3, 4));

// Test nested function calls
function callOtherFunction(a: number, b: number): number {
    return add(a, b) * 2;
}

console.log("\nNested function calls:");
console.log("callOtherFunction(2, 3):", callOtherFunction(2, 3));
console.log("callOtherFunction.call(null, 2, 3):", callOtherFunction.call(null, 2, 3));
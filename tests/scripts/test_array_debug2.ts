let numbers = [1, 2, 3];

// Test simple function first  
function test(x: number): number {
    console.log("test called with:", x);
    return x + 100;
}

console.log("Direct function test:");
let directResult = test(5);
console.log("Direct result:", directResult);

console.log("\nTesting map:");
console.log("Function type:", typeof test);
console.log("Function object:", test);

let mapResult = numbers.map(test);
console.log("Map result:", mapResult);
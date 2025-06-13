let numbers = [1, 2, 3];

// Test simple function first
function test(x: number): number {
    console.log("test called with:", x);
    return x + 100;
}

console.log("Testing direct function call:");
console.log(test(5));

console.log("\nTesting map:");
let result = numbers.map(test);
console.log("Map result:", result);
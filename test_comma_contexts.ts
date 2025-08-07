// Test comma operator in different contexts
function test(a: number, b: number) {
    return a + b;
}

// This should work - comma operator in parentheses as a single argument
let x = test((1, 2), 5); // (1, 2) evaluates to 2, so test(2, 5) = 7
console.log("x:", x);

// This should work - comma operator in assignment
let y = 0;
let z = (y = 3, y + 4); // Should be 7
console.log("y:", y, "z:", z);

// This should work - separate arguments
let w = test(10, 20); // Should be 30
console.log("w:", w);
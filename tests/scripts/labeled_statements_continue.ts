// Test labeled continue statements
// expect: 3

let sum = 0;

// Test labeled continue with nested loops
outer: for (let i = 0; i < 3; i++) {
    for (let j = 0; j < 3; j++) {
        if (j === 1) {
            continue outer;  // Skip to next iteration of outer loop
        }
        sum += i + j;
    }
}

sum;  // Should be 0+0 + 1+0 + 2+0 = 3, but let's verify the actual behavior
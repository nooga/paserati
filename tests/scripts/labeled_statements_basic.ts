// Test basic labeled statements functionality
// expect: outer break success

let result = "";

// Test labeled break with nested loops
outer: for (let i = 0; i < 3; i++) {
    for (let j = 0; j < 3; j++) {
        if (i === 1 && j === 1) {
            result = "outer break success";
            break outer;
        }
        result = "should not reach here";
    }
    result = "should not reach here either";
}

result;
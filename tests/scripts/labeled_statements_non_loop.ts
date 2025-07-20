// Test labeled statements with non-loop statements
// expect: block break success

let result = "";

// Test labeled break with block statement
outer: {
    for (let i = 0; i < 3; i++) {
        if (i === 1) {
            result = "block break success";
            break outer;
        }
    }
    result = "should not reach here";
}

result;
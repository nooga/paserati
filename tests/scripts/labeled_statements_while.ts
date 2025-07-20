// Test labeled statements with while loops
// expect: while break success

let result = "";
let i = 0;
let j = 0;

// Test labeled break with nested while loops
outer: while (i < 3) {
    j = 0;
    while (j < 3) {
        if (i === 1 && j === 1) {
            result = "while break success";
            break outer;
        }
        j++;
    }
    i++;
}

result;
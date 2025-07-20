// Test deeply nested labeled statements
// expect: level 3

let result = "";

outer: for (let i = 0; i < 2; i++) {
    middle: for (let j = 0; j < 2; j++) {
        inner: for (let k = 0; k < 2; k++) {
            if (i === 1 && j === 0 && k === 1) {
                result = "level 3";
                break outer;
            }
        }
    }
}

result;
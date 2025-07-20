// Test labeled statements with for-of loops
// expect: found 2

let result = "";
let items = [1, 2, 3, 4, 5];

outer: for (let item of items) {
    for (let i = 0; i < 3; i++) {
        if (item === 2) {
            result = "found " + item;
            break outer;
        }
    }
}

result;
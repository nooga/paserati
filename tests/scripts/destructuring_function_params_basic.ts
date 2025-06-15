// Test basic function parameter destructuring
function processArray([a, b]: [number, number]) {
    return a + b;
}

let arr: [number, number] = [10, 20];
let result = processArray(arr);
result;
// expect: 30
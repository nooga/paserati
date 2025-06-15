// Test mixed parameters with destructuring
function processData(prefix: string, [first, second]: [number, number]) {
    return prefix + ": " + (first + second);
}

let data: [number, number] = [100, 200];
let result = processData("Sum", data);
result;
// expect: Sum: 300
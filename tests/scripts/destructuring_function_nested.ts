// Test nested destructuring in function parameters
function processData([[a, b], {name, coords: [x, y]}]) {
    return a + b + name + x + y;
}

let result = processData([[1, 2], {name: "test", coords: [10, 20]}]);
result;
// expect: 3test1020
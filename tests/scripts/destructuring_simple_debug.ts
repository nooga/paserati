// Test simple destructuring without types
function test([a, b]) {
    return a + b;
}

let result = test(["hello", "world"]);
result;
// expect: helloworld
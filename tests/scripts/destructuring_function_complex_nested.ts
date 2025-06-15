// Test complex nested destructuring in function parameters
function processComplexData([first, {user: {name, age}, points: [x, y]}, ...rest]) {
    let restSum = 0;
    for (let i = 0; i < rest.length; i++) {
        restSum += rest[i];
    }
    return first + name + age + x + y + restSum;
}

let result = processComplexData([
    100,
    {user: {name: "test", age: 25}, points: [10, 20]},
    5, 15, 25
]);
result;
// expect: 100test25102045
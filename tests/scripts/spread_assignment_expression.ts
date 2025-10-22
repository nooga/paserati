// expect: 5-1-5-3-3
// Test spread operator with assignment expression
// ECMAScript spec: SpreadElement evaluates AssignmentExpression
// Example: [...x = [1, 2]] creates array and assigns to x

let target: any;
let result = [1, 2, ...target = [3, 4, 5]];

result.length + "-" + result[0] + "-" + result[4] + "-" + target.length + "-" + target[0]

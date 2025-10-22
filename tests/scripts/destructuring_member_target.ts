// expect: 42-100
// Test destructuring assignment to member expressions
let obj: any = {};
let arr: any = [];

[obj.x, arr[0]] = [42, 100];

obj.x + "-" + arr[0];

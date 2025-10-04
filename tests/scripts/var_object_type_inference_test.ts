// Test that var object destructuring properly infers types
// expect: Alice25

var {name, age} = {name: "Alice", age: 25};
// name should be string, age should be number
let nameStr: string = name;
let ageNum: number = age;
nameStr + ageNum;

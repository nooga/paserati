// Test object rest elements in const declaration
const {name, ...info} = {name: "John", age: 30, city: "NYC"};
name;
// expect: John
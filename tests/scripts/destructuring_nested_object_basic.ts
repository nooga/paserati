// Test nested object destructuring
let name = "";
let age = 0;
let city = "";

{user: {name, age}, location: {city}} = {
    user: {name: "John", age: 30},
    location: {city: "New York"}
};

let result = name + ":" + age + ":" + city;
result;
// expect: John:30:New York
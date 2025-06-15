// Test nested destructuring in let/const declarations
let [a, [b, c]] = [1, [2, 3]];
const {user: {name, age}, coords: [x, y]} = {
    user: {name: "John", age: 30},
    coords: [10, 20]
};

let result = a + b + c;
let info = name + ":" + age + ":" + x + ":" + y;
let final = result + ":" + info;
final;
// expect: 6:John:30:10:20
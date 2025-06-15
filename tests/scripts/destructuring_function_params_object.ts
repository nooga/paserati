// Test object parameter destructuring
function greet({name, age}) {
    return name + " is " + age + " years old";
}

let person = {name: "Alice", age: 25};
let message = greet(person);
message;
// expect: Alice is 25 years old
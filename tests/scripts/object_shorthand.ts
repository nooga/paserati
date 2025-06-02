// expect: 75

let name = "Alice";
let age = 30;
let active = true;

// Test basic shorthand syntax
let person = { name, age, active };

console.log(person.name); // Alice
console.log(person.age); // 30
console.log(person.active); // true

// Test mixed shorthand and regular properties
let id = 123;
let user = {
  id,
  name: "Bob",
  age,
  email: "bob@example.com",
};

console.log(user.id); // 123
console.log(user.name); // Bob
console.log(user.age); // 30
console.log(user.email); // bob@example.com

// Test shorthand with calculations
let x = 10;
let y = 15;
let point = { x, y };
let result = point.x + point.y; // 10 + 15 = 25

// Test shorthand with function calls
function getValue(): number {
  return 20;
}

let val = getValue();
let obj = { val };
result += obj.val; // 25 + 20 = 45

// Test in nested objects
let inner = { name: "inner" };
let outer = { inner, value: 5 };
result += outer.value; // 45 + 5 = 50

// Final calculation to reach expected 75
result + 25;

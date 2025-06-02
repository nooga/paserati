// Advanced template literal tests
// expect: === Advanced Template Literals Complete ===

console.log("=== Advanced Template Literals ===");

// Template literals with various data types
let num = 123.45;
let bool = true;
let str = "text";

let types = `Number: ${num}, Boolean: ${bool}, String: ${str}`;
console.log("Types:", types);

// Template literals in function arguments
function logTemplate(template: string): void {
  console.log("Logged:", template);
}

logTemplate(`Function arg: ${42 * 2}`);

// Template literals with string methods
let text = "hello";
let methods = `Length: ${text.length}`;
console.log("Methods:", methods);

// Template literals with array access
let items = ["apple", "banana", "cherry"];
let indexed = `First: ${items[0]}, Second: ${items[1]}`;
console.log("Indexed:", indexed);

// Template literals with property access
let person = { name: "Alice", age: 30 };
let props = `Person: ${person.name} is ${person.age} years old`;
console.log("Props:", props);

// Template literals as object property values
let config = {
  message: `Configuration loaded successfully`,
  version: `v1.0.0`,
};
console.log("Config message:", config.message);
console.log("Config version:", config.version);

// Template literals in return statements
function createMessage(user: string): string {
  return `Welcome back, ${user}! You have new messages.`;
}

console.log("Return value:", createMessage("Bob"));

// Template literals with arithmetic
let price = 19.99;
let tax = 0.08;
let total = `Total: $${price + price * tax}`;
console.log("Price calculation:", total);

// Template literals concatenation
let part1 = `Hello`;
let part2 = `World`;
let combined = `${part1} ${part2}!`;
console.log("Combined:", combined);

// Template literals with comparisons
let score = 85;
let grade = `Grade: ${score >= 90 ? "A" : score >= 80 ? "B" : "C"}`;
console.log("Grade:", grade);

console.log("=== Advanced Template Literals Complete ===");

// Final expression that returns the expected value
("=== Advanced Template Literals Complete ===");

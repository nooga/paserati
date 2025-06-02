// Basic template literal tests
// expect: === Template Literals Complete ===

console.log("=== Basic Template Literals ===");

// Simple template literal with no interpolation
let simple = `hello world`;
console.log("Simple:", simple);

// Template literal with variable interpolation
let name = "TypeScript";
let greeting = `Hello, ${name}!`;
console.log("Greeting:", greeting);

// Template literal with expression interpolation
let x = 10;
let y = 5;
let result = `${x} + ${y} = ${x + y}`;
console.log("Math:", result);

// Template literal with function call interpolation
function getName(): string {
  return "Paserati";
}
let funcCall = `Project: ${getName()}`;
console.log("Function call:", funcCall);

// Template literal with complex expressions
let value = 42;
let complex = `Value is ${value > 40 ? "large" : "small"} (${value})`;
console.log("Complex:", complex);

// Multiple interpolations in one template
let a = 1;
let b = 2;
let c = 3;
let multiple = `Values: ${a}, ${b}, ${c}`;
console.log("Multiple:", multiple);

// Empty template literal
let empty = ``;
console.log("Empty length:", empty.length);

// Template with just expressions
let justExpr = `${100}`;
console.log("Just expression:", justExpr);

console.log("=== Template Literals Complete ===");

// Final expression that returns the expected value
("=== Template Literals Complete ===");

// Type annotations that should be stripped
let x: number = 42;
const y: string = "Hello";
var z: boolean = true;

// Function with type annotations
function add(a: number, b: number): number {
  return a + b;
}

// Arrow function
const multiply = (a: number, b: number): number => a * b;

// Object with types
interface Person {
  name: string;
  age: number;
}

const person = {
  name: "Alice",
  age: 30,
};

// Array with type
const numbers: number[] = [1, 2, 3, 4, 5];

// Conditional logic
if (x > 40) {
  console.log("x is greater than 40");
} else {
  console.log("x is not greater than 40");
}

// Loops
for (let i = 0; i < 5; i++) {
  console.log(i);
}

let j = 0;
while (j < 3) {
  console.log(j);
  j++;
}

// Ternary operator
const isAdult = person.age >= 18 ? true : false;

// Function expression
const factorial = function (n: number): number {
  if (n <= 1) return 1;
  return n * factorial(n - 1);
};

// Call the functions
console.log(add(x, 10));
console.log(multiply(5, 6));
console.log(factorial(5));

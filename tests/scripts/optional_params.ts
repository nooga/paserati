// Test file for optional parameters
// expect: true

// Basic optional parameter
function greet(name: string, greeting?: string): string {
  if (greeting === undefined) {
    return "Hello, " + name;
  }
  return greeting + ", " + name;
}

// Multiple optional parameters
function createUser(name: string, age?: number, email?: string): string {
  let result = "User: " + name;
  if (age !== undefined) {
    result += ", Age: " + age;
  }
  if (email !== undefined) {
    result += ", Email: " + email;
  }
  return result;
}

// Arrow function with optional parameter
let multiply = (a: number, b?: number): number => {
  if (b === undefined) {
    return a * a; // Square if no second argument
  }
  return a * b;
};

// Test all cases and return true if they all pass
greet("Alice") === "Hello, Alice" &&
  greet("Bob", "Hi") === "Hi, Bob" &&
  createUser("Alice") === "User: Alice" &&
  createUser("Bob", 25) === "User: Bob, Age: 25" &&
  createUser("Charlie", 30, "c@test.com") ===
    "User: Charlie, Age: 30, Email: c@test.com" &&
  multiply(5) === 25 &&
  multiply(3, 5) === 15;

// Optional parameters must come after required ones (type checking should enforce this)
// This should cause a type error when we implement validation:
// function invalid(optional?: string, required: number): void {}

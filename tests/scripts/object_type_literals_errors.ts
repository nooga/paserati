// Test file for object type literal error cases
// expect: error

// Type mismatch: expected number, got string
let person1: { name: string; age: number } = { name: "John", age: "thirty" };

// Missing required property
let person2: { name: string; age: number } = { name: "Jane" };

// Extra property
let person3: { name: string; age: number } = {
  name: "Bob",
  age: 25,
  extra: "value",
};

// Wrong property type in nested object
let nestedError: { user: { id: number; name: string } } = {
  user: { id: "not-a-number", name: "Alice" },
};

// Function parameter type mismatch
function processUser(user: { id: number; name: string }): string {
  return user.name;
}
let errorResult: string = processUser({ id: "123", name: "Charlie" });

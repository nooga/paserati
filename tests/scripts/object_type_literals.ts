// Test file for object type literals
// expect: {x: 10, y: 20}

// Basic object type literal
let person: { name: string; age: number } = { name: "John", age: 30 };

// Empty object type
let empty: {} = {};

// Mixed types
let mixed: { count: number; active: boolean; label: string } = {
  count: 42,
  active: true,
  label: "test",
};

// Nested object types
let nested: {
  user: { id: number; name: string };
  settings: { theme: string };
} = {
  user: { id: 1, name: "Alice" },
  settings: { theme: "dark" },
};

// Object type in function parameter
function processUserValid(user: { id: number; name: string }): string {
  return user.name + " (" + user.id + ")";
}

let userResult: string = processUserValid({ id: 123, name: "Bob" });

// Object type as return type
function createPoint(): { x: number; y: number } {
  return { x: 10, y: 20 };
}

let point: { x: number; y: number } = createPoint();

point;

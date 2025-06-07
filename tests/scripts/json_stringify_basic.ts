// Test JSON.stringify with basic types
let numJson = JSON.stringify(42);
let strJson = JSON.stringify("hello");
let boolJson = JSON.stringify(true);
let nullJson = JSON.stringify(null);

let basicCorrect =
  numJson === "42" &&
  strJson === '"hello"' &&
  boolJson === "true" &&
  nullJson === "null";

// expect: true
basicCorrect;

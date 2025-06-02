// Test file for console.log functionality
// expect: ok

// Test all console methods
console.log("=== Console Test Suite ===");

// Basic logging methods
console.log("Standard log message");
console.error("Error message");
console.warn("Warning message");
console.info("Info message");
console.debug("Debug message");

// Complex data structures
console.log("Object:", {
  name: "John",
  age: 30,
  hobbies: ["reading", "coding"],
});
console.log("Array:", [1, "two", { three: 3 }, [4, 5]]);

// Counting
console.count("operation");
console.count("operation");
console.count("special");
console.count("operation");

// Timing
console.time("process");
console.time("subtask");
console.timeEnd("subtask");
console.timeEnd("process");

// Grouping
console.group("Main Group");
console.log("Item 1 in main group");
console.log("Item 2 in main group");
console.group("Nested Group");
console.log("Item in nested group");
console.groupEnd();
console.log("Back in main group");
console.groupEnd();

console.log("=== Test Complete ===");
("ok");

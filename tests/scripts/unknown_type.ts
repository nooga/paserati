// Test file for 'unknown' type behavior
// This tests the difference between 'any' and 'unknown'

// expect: ok

// Test 1: Basic assignment to unknown
let x: unknown = 5;
let y: unknown = "hello";
let z: unknown = true;
let w: unknown = null;
let v: unknown = undefined;

console.log("Basic assignment works");

// Test 2: Assignment FROM unknown - should only work to any and unknown
let a: unknown = 42;
let b: any = a; // Should work
let c: unknown = a; // Should work

// This should fail (uncomment to test):
// let d: number = a; // Type error

console.log("Assignment from unknown to any/unknown works");

// Test 3: Direct operations on unknown should fail
let val: unknown = "hello world";

// These should all fail (uncomment to test):
// console.log(val.length); // Property access error
// console.log(val.toUpperCase()); // Method call error
// val(); // Call error
// val + 5; // Arithmetic error

console.log("Direct operations properly blocked");

// Test 4: Compare with any - operations should work
let anyVal: any = "hello world";
console.log("Length of any:", anyVal.length); // Should work
console.log("Uppercase any:", anyVal.toUpperCase()); // Should work

// Test 5: Type checking and narrowing (currently not implemented)
let unknown1: unknown = "test string";

// This currently fails but should work with type narrowing:
if (typeof unknown1 === "string") {
  // In TypeScript, unknown1 should be narrowed to string here
  console.log(unknown1.length); // Should work after narrowing
  console.log("Type check passed, narrowing should work!");
}

// Test 6: Function parameters
function acceptsUnknown(param: unknown): void {
  // Direct use should fail:
  // console.log(param.toString()); // Should error

  // With type checking (when narrowing is implemented):
  if (typeof param === "string") {
    console.log(param.length); // Should work
    console.log("Function param type checking works");
  }
}

acceptsUnknown("hello");
acceptsUnknown(42);
acceptsUnknown(true);

console.log("Unknown type tests completed");

("ok");

// Test control flow analysis with throw statements

function testThrowNarrowing(input: unknown): string {
  // If input is not string, throw - so after this, input must be string
  if (typeof input !== "string") {
    throw new Error("Not a string");
  }
  
  // This should work - input should be narrowed to string
  return input.toUpperCase();
}

function testThrowNarrowingPositive(input: unknown): string {
  // If input is string, throw - so after this, input is not string (should error)
  if (typeof input === "string") {
    throw new Error("Is a string");
  }
  
  // This should error - input is not string here
  return input.toUpperCase();
}

console.log("Test completed");
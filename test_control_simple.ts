// Simple control flow test
function test(input: unknown): string {
  if (typeof input !== "string") {
    throw new Error("Not a string");
  }
  
  // Should work - input is string here
  return input;
}
// Test string method access after narrowing
function testStringMethods(input: unknown): void {
  if (typeof input === "string") {
    const upper = input.toUpperCase(); // Should work
    const replaced = input.replace("a", "b"); // Should work
    console.log(upper, replaced);
  }
}
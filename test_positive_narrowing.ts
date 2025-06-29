// Test positive type narrowing (should work)
function testPositiveNarrowing(input: unknown): void {
  if (typeof input === "string") {
    const upper = input.toUpperCase(); // Should work
    console.log(upper);
  }
}

// Test parseInt
const num = parseInt("42");
console.log(num);
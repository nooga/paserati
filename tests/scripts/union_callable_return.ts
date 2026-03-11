// Test calling a union of function types and getting a union return type
// This covers the pattern: string.includes | Uint8Array.includes

function test(): string {
  const arr: (string | number)[] = ["hello", 42];
  const results: string[] = [];
  for (const item of arr) {
    results.push(item.toString());
  }
  return results.join(",");
}

test();

// expect: hello,42

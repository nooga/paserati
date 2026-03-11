// expect: default
// Test: assignment in if-block narrows variable type after the block
// if (x === null) { x = "default"; } should narrow x to string after the if

function test(x: string | null): string {
  if (x === null) {
    x = "default";
  }
  // After if: x should be string (union of then-branch string + else-branch string)
  return x;
}

test(null);

// Test multi-line parameters
function test(
  a: number,
  b: number = 5
): number {
  return a + b;
}

test(3);
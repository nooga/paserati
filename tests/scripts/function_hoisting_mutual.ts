// mutual recursion across declarations to validate hoisting order
// expect: 8

function a(n: number): number {
  return n === 0 ? 0 : b(n - 1) + 1;
}
function b(n: number): number {
  return n === 0 ? 0 : a(n - 1) + 1;
}

a(8);

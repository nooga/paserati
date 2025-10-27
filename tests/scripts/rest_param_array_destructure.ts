// Test rest parameter with array destructuring
// no-typecheck
// expect: 6

function f(...[x, y, z]) {
  return x + y + z;
}

f(1, 2, 3, 4, 5);

// Test rest parameter with nested destructuring and defaults
// no-typecheck
// expect: 1,2,default,4

function f(...[x, y, z = "default", w]) {
  return `${x},${y},${z},${w}`;
}

f(1, 2, undefined, 4);

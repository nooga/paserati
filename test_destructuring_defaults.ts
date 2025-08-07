// Test destructuring parameter defaults
function test1({x = 42}: {x?: number} = {}) {
  return x;
}

function test2([a = 10, b = 20]: number[] = []) {
  return a + b;
}
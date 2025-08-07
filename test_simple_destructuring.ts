// Simple destructuring parameter default test - no execution, just parsing
function test1({x = 10}: {x?: number} = {}) {
  return x;
}

function test2([a = 1, b = 2]: number[] = []) {
  return a + b;
}
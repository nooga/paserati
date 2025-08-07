// Test various destructuring parameter default patterns

// Object destructuring with top-level default
function test1({x = 10}: {x?: number} = {}) {
  return x;
}

// Array destructuring with top-level default
function test2([a = 1, b = 2]: number[] = []) {
  return a + b;
}

// Mixed destructuring patterns
function test3({name = "unknown"}: {name?: string} = {}) {
  return name;
}

console.log("Test 1:", test1());           // Should print 10
console.log("Test 2:", test2());           // Should print 3
console.log("Test 3:", test3());           // Should print "unknown"
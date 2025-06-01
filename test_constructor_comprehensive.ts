// Test 1: Basic constructor
function Point(x, y) {
  // Simple assignment without type checking issues
}

let p1 = new Point(10, 20);

// Test 2: Constructor with explicit return of primitive (should return instance)
function Test1() {
  if (false) return 42; // Conditional return to avoid type checker issues
}

let t1 = new Test1();

// Test 3: Constructor with explicit return of object (should return that object)
function Test2() {
  if (false) return {}; // Conditional return to avoid type checker issues
}

let t2 = new Test2();

// Test 4: Constructor with no explicit return (should return instance)
function Test3() {
  // No return statement
}

let t3 = new Test3();

// Output the results
p1;
t1;
t2;
t3;

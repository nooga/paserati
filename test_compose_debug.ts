// Debug the compose function issue

// Let's break down the compose function step by step

// Step 1: Simple case - return a non-generic function
function test1(): (x: number) => number {
  return (x: number) => x * 2;  // Should work
}

// Step 2: Generic function returning non-generic function  
function test2<T>(): (x: number) => number {
  return (x: number) => 42;  // Should work
}

// Step 3: Generic function returning generic function with same T
function test3<T>(): (x: T) => T {
  return (x: T) => x;  // This fails - why?
}

// Step 4: The actual compose pattern
function compose<T, U, V>(f: (x: U) => V, g: (x: T) => U): (x: T) => V {
  // Let's see if we can work around it by avoiding the arrow function
  function inner(x: T): V {
    return f(g(x));
  }
  return inner;  // Does this work?
}

console.log("Debug tests");

// expect: Debug tests
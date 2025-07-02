// Comprehensive test for default type parameter constraint bug fix
// Testing various scenarios where default type parameters reference earlier type parameters

// Test 1: Basic case - T extends Date, U extends Date = T
function test1<T extends Date, U extends Date = T>(a: T, b?: U): T {
  return a;
}

// Test 2: Different constraints - T extends string, U extends string = T
function test2<T extends string, U extends string = T>(a: T): U {
  return a as any;
}

// Test 3: Complex constraint - T extends { length: number }, U extends { length: number } = T
function test3<T extends { length: number }, U extends { length: number } = T>(a: T): U {
  return a as any;
}

// Test 4: Interface constraint - T extends Foo, U extends Foo = T
interface Foo {
  name: string;
}

function test4<T extends Foo, U extends Foo = T>(a: T): U {
  return a as any;
}

// All tests should compile without constraint errors
console.log("All default type parameter constraint tests passed");

// expect: undefined
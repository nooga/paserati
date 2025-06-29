// Test generic function return type

// Simplest case - generic function returning its type parameter
function test1<T>(): T {
  return undefined as any as T;
}

// Return a function with generic types
function test2<T>(): (x: T) => T {
  return (x: T) => x;
}

// This is the exact pattern from compose
function test3<T, V>(): (x: T) => V {
  const fn = (x: T): V => {
    return undefined as any as V;
  };
  return fn;
}

// Even simpler - no generics in the return
function test4(): (x: number) => number {
  return (x: number) => x * 2;
}

console.log("Tests completed");

// expect: Tests completed
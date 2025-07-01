// Test default type parameters in type aliases - should now work

type Test<T = string> = T;

// expect: null
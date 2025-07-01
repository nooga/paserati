// Test default type parameters in classes - should now work

class Test<T = string> {
    value: T;
}

// expect: null
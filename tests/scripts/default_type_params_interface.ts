// Test default type parameters in interfaces - should now work

interface Container<T = string> {
    value: T;
}

// expect: undefined
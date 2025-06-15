// Test generic type error - wrong arity

interface Container<T> {
    value: T;
}

// This should be an error - wrong number of type arguments
let badContainer: Container<string, number>;

badContainer;

// expect: undefined
// Test generic type error

interface Container<T> {
    value: T;
}

let stringContainer: Container<string>;

// This should be an error - wrong number of type arguments
let badContainer: Container;

badContainer;

// expect: undefined
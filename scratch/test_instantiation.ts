// Test generic interface instantiation

interface Container<T> {
    value: T;
}

let stringContainer: Container<string>;
let numberContainer: Container<number>;

stringContainer;
numberContainer;

// expect: undefined
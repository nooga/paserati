// Test various type errors with generics

interface Container<T> {
    value: T;
}

type Optional<T> = T | undefined;

let stringContainer: Container<string>;
let numberOptional: Optional<number>;

// This should error: wrong type
let wrongType: Container<string> = { value: 123 };

wrongType;

// expect: undefined
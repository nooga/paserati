// Test comprehensive generic type checking

interface Container<T> {
    value: T;
    getValue(): T;
}

type Optional<T> = T | undefined;

// This should work
let stringContainer: Container<string>;
let optionalNumber: Optional<number>;

// These should be type errors
let wrongAssignment: Container<string> = { value: 42 }; // Should error: number not assignable to string

wrongAssignment;

// expect: undefined
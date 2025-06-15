// Test just defining generic interfaces without using them

interface Container<T> {
    value: T;
    getValue(): T;
}

interface Pair<T, U> {
    first: T;
    second: U;
}

undefined; // Explicit undefined expression

// expect: undefined
// Test generic interface inheritance (simplified - extends non-generic for now)

interface Readable {
    read(): string;
}

interface Container<T> extends Readable {
    value: T;
    // Inherits read(): string
}

let stringContainer: Container<string>;
stringContainer;

// expect: undefined
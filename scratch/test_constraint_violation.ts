// Test constraint violation

interface Lengthable {
    length: number;
}

interface Container<T extends Lengthable> {
    item: T;
}

// This should error - number doesn't have length property
let badContainer: Container<number> = {
    item: 42
};

badContainer;

// expect: undefined
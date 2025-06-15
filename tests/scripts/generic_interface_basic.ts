// Test basic generic interface declaration and usage

interface Container<T> {
    value: T;
    getValue(): T;
}

interface Pair<T, U> {
    first: T;
    second: U;
}

let stringContainer: Container<string>;
let numberPair: Pair<number, string>;

stringContainer;
numberPair;

// expect: undefined
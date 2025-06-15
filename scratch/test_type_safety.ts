// Test generic type safety

interface Container<T> {
    value: T;
    getValue(): T;
}

type Optional<T> = T | undefined;

let stringContainer: Container<string>;
let optionalNumber: Optional<number>;

// This should work
let str: string = "hello";
let num: number = 42;

// Test that instantiated types are correctly typed
optionalNumber = 42;        // Should work
optionalNumber = undefined; // Should work  

str;
num;
optionalNumber;

// expect: undefined
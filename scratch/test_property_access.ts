// Test property access on generic types

interface Container<T> {
    value: T;
    getValue(): T;
}

let stringContainer: Container<string> = { 
    value: "hello",
    getValue: () => "world"
};

// Test property access
let val = stringContainer.value; // Should be string
let result = stringContainer.getValue(); // Should be string

val;
result;

// expect: world
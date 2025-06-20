// Test generic constraints

interface Lengthable {
    length: number;
}

interface Container<T extends Lengthable> {
    item: T;
    getLength(): number;
}

// This should work - string has length
let stringContainer: Container<string> = {
    item: "hello",
    getLength: () => 5
};

// This should work - arrays have length  
let arrayContainer: Container<Array<number>> = {
    item: [1, 2, 3],
    getLength: () => 3
};

stringContainer.item;
arrayContainer.item;

// expect: 1,2,3
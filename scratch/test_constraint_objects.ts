// Test constraint validation with object types

interface Lengthable {
    length: number;
}

interface Container<T extends Lengthable> {
    item: T;
}

// This should work - object with length property
let goodContainer: Container<{length: number; value: string}> = {
    item: {length: 5, value: "hello"}
};

// This should fail - object without length property  
let badContainer: Container<{value: string}> = {
    item: {value: "hello"}
};

goodContainer.item.length;

// expect: 5
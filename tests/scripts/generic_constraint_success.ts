// Test successful constraint validation

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

goodContainer.item.length;

// expect: 5
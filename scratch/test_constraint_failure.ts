// Test constraint validation failure

interface Lengthable {
    length: number;
}

interface Container<T extends Lengthable> {
    item: T;
}

// This should fail - object without length property  
let badContainer: Container<{value: string}> = {
    item: {value: "hello"}
};

// expect_compile_error: Type '{value: string}' does not satisfy constraint '{length: number}' for type parameter 'T'
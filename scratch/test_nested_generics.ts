// Test nested generic types

interface Container<T> {
    value: T;
}

type NestedContainer<T> = Container<Container<T>>;

let nested: NestedContainer<string> = {
    value: {
        value: "hello"
    }
};

let innerValue = nested.value.value; // Should be string
innerValue;

// expect: hello
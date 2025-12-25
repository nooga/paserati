// Test mixed default and non-default type parameters

// Test interface with mixed defaults
interface MixedContainer<T, U = boolean> {
    first: T;
    second: U;
}

let container1: MixedContainer<string>; // U should default to boolean
let container2: MixedContainer<string, number>; // Explicit U

// Test type alias with mixed defaults
type MixedType<T, U = string> = {
    first: T;
    second: U;
};

let mixed1: MixedType<number>; // U should default to string
let mixed2: MixedType<number, boolean>; // Explicit U

// expect: undefined
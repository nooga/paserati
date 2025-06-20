// Test return type checking with generics

interface Converter<T, U> {
    convert(input: T): U;
}

let converter: Converter<string, number> = {
    // This should error - returning string instead of number
    convert: (s: string) => s
};

converter;

// expect: undefined
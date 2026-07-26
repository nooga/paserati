// Test return type checking with generics

interface Converter<T, U> {
    convert(input: T): U;
}

let converter: Converter<string, number> = {
    // This should error - returning string instead of number
    convert: (s: string) => s
};

// expect_compile_error: Type '{ convert: (string) => string }' is not assignable to type '{ convert: (string) => number }'.
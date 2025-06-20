// Test return type checking with generics

interface Converter<T, U> {
    convert(input: T): U;
}

let converter: Converter<string, number> = {
    // This should error - returning string instead of number
    convert: (s: string) => s
};

// expect_compile_error: cannot assign type '{ convert: (string) => string }' to variable 'converter' of type '{ convert: (string) => number }'
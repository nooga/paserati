// Test function signature substitution in generics

interface Processor<T> {
    process(input: T): T;
    validate(input: T): boolean;
}

let stringProcessor: Processor<string> = {
    process: (s: string) => s.toUpperCase(),
    validate: (s: string) => s.length > 0
};

// This should work - correct types
let result = stringProcessor.process("hello");
let isValid = stringProcessor.validate("test");

// This should error - wrong parameter type
let badResult = stringProcessor.process(123);

result;
isValid;
badResult;

// expect: true
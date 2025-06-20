// Test simple function without built-in methods

interface Processor<T> {
    process(input: T): T;
}

let stringProcessor: Processor<string> = {
    process: (s: string) => s // Just return input
};

let result = stringProcessor.process("hello");
result;

// expect: hello
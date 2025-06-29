// Test simple generic callback with type parameter calls
// expect: 42

function withGeneric<T extends (x: number) => number>(fn: T, callback: (f: T) => number): number {
    return callback(fn);
}

function double(x: number): number {
    return x * 2;
}

// This should work - f should be callable because T extends (x: number) => number
const result = withGeneric(double, (f) => f(21));

result;
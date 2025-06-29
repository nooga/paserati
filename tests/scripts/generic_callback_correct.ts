// Test proper generic callback usage
// expect: 6

function withCallback<T extends (n: number) => number>(
    fn: T,
    callback: (f: T) => number
): number {
    return callback(fn);
}

function factorial(n: number): number {
    return n === 0 ? 1 : n * factorial(n - 1);
}

const result = withCallback(factorial, (f) => f(3));

result;
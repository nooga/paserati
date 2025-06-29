// Test generic type parameter resolution in callbacks
// expect: 42

// Simple generic function that passes the type parameter to a callback
function withCallback<T>(value: T, callback: (x: T) => number): number {
    return callback(value);
}

// Test with function type
type MyFunc = (n: number) => number;
const double: MyFunc = (n) => n * 2;

// This should work - the callback receives a MyFunc and should be able to call it
const result = withCallback<MyFunc>(double, (f) => f(21));

result;
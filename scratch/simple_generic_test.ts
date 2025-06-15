// Test simple generic function calls

function identity<T>(x: T): T {
    return x;
}

let str = identity("hello");
let num = identity(42);

str;
num;

// expect: 42
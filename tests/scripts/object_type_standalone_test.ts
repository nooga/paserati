// expect: object type standalone test

// Test object types outside of conditional types

// This should work - object type alias with multiple properties
type MyObject = { a: string, b: number };

// This should work - using the type
function test() {
    let obj: MyObject = { a: "hello", b: 42 };
    return obj;
}

test();

"object type standalone test";
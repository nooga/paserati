// expect: object type debug test

// Test 1: Basic object type literal parsing
type SimpleObject<T> = T extends { a: string, b: number } ? "match" : "no match";

// Test 2: Object type with type parameters
type ObjectWithParams<T, U> = T extends { a: U, b: string } ? "match" : "no match";

// Test 3: Object type in conditional - this should work
type TestObj = { a: string, b: number };

function test() {
    type Result1 = SimpleObject<TestObj>; // Should be "match"
    type Result2 = ObjectWithParams<TestObj, string>; // Should be "no match" since TestObj.a is string, not U=string
    
    let val1: Result1 = "match";
    let val2: Result2 = "no match";
    
    return val1;
}

test();

"object type debug test";
// expect_compile_error: cannot assign type 'string' to variable 'num' of type 'number'

// Test that type predicate narrowing correctly detects type errors

function isString(x: any): x is string {
    return typeof x === "string";
}

let value: any = 42;

if (isString(value)) {
    // Inside this block, value should be narrowed to string
    // This assignment should fail: can't assign string to number
    let num: number = value;
} else {
    console.log("not a string");
}
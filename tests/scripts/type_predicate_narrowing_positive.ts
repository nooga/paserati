// expect: narrowed successfully

// Test that type predicate narrowing works correctly for valid cases

function isString(x: any): x is string {
    return typeof x === "string";
}

function isNumber(x: any): x is number {
    return typeof x === "number";
}

let value: any = 42;

let result: string;
if (isNumber(value)) {
    // Inside this block, value should be narrowed to number
    // This assignment should work without type errors
    let num: number = value;
    result = "narrowed successfully";
} else {
    result = "not narrowed";
}

result;
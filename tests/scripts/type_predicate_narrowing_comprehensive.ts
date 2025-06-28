// expect: string test passed, number test passed

// Test comprehensive type predicate narrowing with multiple types

function isString(x: any): x is string {
    return typeof x === "string";
}

function isNumber(x: any): x is number {
    return typeof x === "number";
}

// Test string narrowing
let stringValue: any = "hello";
let stringResult: string;
if (isString(stringValue)) {
    // Should narrow to string
    let str: string = stringValue;
    stringResult = "string test passed";
} else {
    stringResult = "string test failed";
}

// Test number narrowing  
let numberValue: any = 42;
let numberResult: string;
if (isNumber(numberValue)) {
    // Should narrow to number
    let num: number = numberValue;
    numberResult = "number test passed";
} else {
    numberResult = "number test failed";
}

stringResult + ", " + numberResult;
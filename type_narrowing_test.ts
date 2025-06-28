// Test type narrowing with type predicates

function isString(x: any): x is string {
    return typeof x === "string";
}

function isNumber(x: any): x is number {
    return typeof x === "number";
}

let value: any = 42;

if (isNumber(value)) {
    // This should work without type errors if narrowing works
    let num: number = value;
    console.log("narrowed to number: " + num);
} else {
    console.log("not a number");
}

// Test string narrowing too
let strValue: any = "hello";
if (isString(strValue)) {
    // This should work without type errors if narrowing works
    let str: string = strValue;
    console.log("narrowed to string: " + str);
}
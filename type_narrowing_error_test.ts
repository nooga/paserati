// Test type narrowing error cases

function isString(x: any): x is string {
    return typeof x === "string";
}

let value: any = 42;

if (isString(value)) {
    // This should cause a type error if narrowing works correctly
    // because inside this block, value should be narrowed to string
    // but we're trying to assign it to number
    let num: number = value;  // Should error: can't assign string to number
    console.log("This shouldn't work: " + num);
} else {
    console.log("correctly not a string");
}
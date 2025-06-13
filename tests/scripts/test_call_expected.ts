function myFunc(x: number): number {
    return x * 2;
}

// Test direct call first to verify expected result
let directResult = myFunc(5);
console.log("Direct call result:", directResult);

// This should return the same thing
// let callResult = myFunc.call(null, 5);
// console.log("Function.call result:", callResult);
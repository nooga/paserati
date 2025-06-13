function myFunc(x: number): number {
    return x * 2;
}

// Test what myFunc actually is
console.log("myFunc:", myFunc);
console.log("typeof myFunc:", typeof myFunc);

// Test accessing .call property
console.log("myFunc.call:", myFunc.call);
console.log("typeof myFunc.call:", typeof myFunc.call);

// Test simple function call first
console.log("Direct call result:", myFunc(5));
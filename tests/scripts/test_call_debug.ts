function myFunc(x: number): number {
    return x * 2;
}

// Test if we can access .call property
console.log("myFunc:", myFunc);
console.log("typeof myFunc:", typeof myFunc);

// Test property access
let callMethod = myFunc.call;
console.log("callMethod:", callMethod);
console.log("typeof callMethod:", typeof callMethod);

// Simple call
console.log("Simple call result:", myFunc(5));
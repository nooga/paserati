function myFunc(x: number): number {
    return x * 2;
}

// Test Function.prototype.call
console.log("Testing Function.prototype.call:");
console.log("myFunc.call(null, 5):", myFunc.call(null, 5));

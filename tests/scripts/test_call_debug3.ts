function myFunc(x: number): number {
    return x * 2;
}

// Test without console.log to see if the issue is with console.log
let result = myFunc.call(null, 5);
// Don't call console.log here yet
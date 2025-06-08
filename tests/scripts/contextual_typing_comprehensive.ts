// Test comprehensive contextual typing scenarios
// expect_compile_error: A spread argument must either have a tuple type or be passed to a rest parameter

function sum3(a: number, b: number, c: number): number {
    return a + b + c;
}

function sumVariadic(...args: number[]): number {
    return args.reduce((sum, n) => sum + n, 0);
}

// Case 1: Direct array literal in spread (should work in TypeScript - contextual typing)
let result1 = sum3(...[1, 2, 3]);

// Case 2: Array variable (should error - unknown length)
let arr = [1, 2, 3];
let result2 = sum3(...arr);

// Case 3: Explicitly typed tuple (should work)
let tuple: [number, number, number] = [1, 2, 3];
let result3 = sum3(...tuple);

// Case 4: Variadic function with array (should work)
let result4 = sumVariadic(...arr);

// Case 5: Variadic function with direct array literal (should work)
let result5 = sumVariadic(...[1, 2, 3, 4, 5]);

result1;
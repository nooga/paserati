// Test .length property on arrays and strings

let arr: number[] = [10, 20, 30, 40];
let str: string = "hello";

let arrLen: number = arr.length; // Should be 4
let strLen: number = str.length; // Should be 5

// Return a combination of the lengths for testing
arrLen * 10 + strLen;

// expect: 45

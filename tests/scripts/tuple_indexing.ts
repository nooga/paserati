// Test tuple indexing - accessing tuple elements by literal index
// expect: hello

// Basic tuple indexing
let tuple: [number, string, boolean] = [42, "hello", true];

// Access each element - should get the specific type
let num: number = tuple[0];
let str: string = tuple[1];
let bool: boolean = tuple[2];

// Return the string element to verify indexing works
str;

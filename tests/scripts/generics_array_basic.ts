// Test basic Array<T> syntax
let arr1: Array<string> = ["hello", "world"];
let arr2: Array<number> = [1, 2, 3];
let arr3: Array<boolean> = [true, false];

// Test that it's the same as array syntax
let arr4: string[] = arr1;
let arr5: Array<string> = arr4;

// expect: ok
("ok");

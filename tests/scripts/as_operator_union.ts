// expect: hello

// Test type assertion with union types
let x: string | number = "hello";
let str = x as string;
str;
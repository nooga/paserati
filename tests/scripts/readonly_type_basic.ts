// Test that Readonly<T> is recognized as a valid type
let x: Readonly<any>;
let y: Readonly<any> | undefined = undefined;

// The type is recognized
console.log("Readonly<T> type is recognized!");
"Readonly<T> type is recognized!"; // Final expression

// expect: Readonly<T> type is recognized!
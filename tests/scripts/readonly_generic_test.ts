// Test Readonly<T> generic type
type X = { a: number };
let y: Readonly<X> | undefined = undefined;

// Should compile without error
console.log("Readonly<T> type works!");
"Readonly<T> type works!"; // Final expression

// expect: Readonly<T> type works!
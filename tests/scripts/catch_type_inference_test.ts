// Test that catch parameters are typed as 'any' and work with type system
// expect: 1

let result: string = "";
try {
    throw [1, 2];
} catch ([x, y]) {
    // Catch parameters are 'any' so this should work
    result = x;
}
result;

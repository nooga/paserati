// expect: object type simple test

// Test: Simple object type literal parsing - now fixed!

// Single property object
type Test1<T> = T extends { a: string } ? "match" : "no match";

// Multiple properties with comma separators  
type Test2<T> = T extends { a: string, b: number } ? "match" : "no match";

"object type simple test";
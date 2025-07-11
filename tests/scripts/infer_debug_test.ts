// expect: infer debug test

// Debug infer step by step

// Test 1: Does the parser handle infer R correctly?
type Test1<T> = T extends infer R ? string : never;

"infer debug test";
// expect: 42

// Test chained type assertions
let x: unknown = 42;
let result = ((x as any) as number);
result;
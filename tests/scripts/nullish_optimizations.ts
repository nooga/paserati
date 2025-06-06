// Test file to showcase all the nullish check optimizations
// This tests that our new efficient opcodes work correctly

// Nullish coalescing operator (??): 1 register instead of 4+
let x: number | null = null;
let y: string | undefined = undefined;
let z = x ?? 42;
let w = y ?? "default";

// Nullish coalescing assignment (??=): 1 register instead of 4+
let a: number | null = null;
a ??= 100;

// Optional chaining (?.): 1 register instead of 5+
let obj: { prop?: string } | null = null;
let value = obj?.prop;

// Strict equality with null/undefined literals: 1 register instead of 3
let result1 = x === null;
let result2 = y === undefined;
let result3 = z !== null;
let result4 = w !== undefined;

// Complex combinations that benefit from all optimizations
let complexChain = obj?.prop ?? "fallback";
let isValid = obj?.prop !== undefined && obj?.prop !== null;

// expect: 42
z;

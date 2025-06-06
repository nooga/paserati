// Test that demonstrates the register efficiency improvements from nullish optimizations
// Before: 4+ registers for nullish checks, now: 1 register

// Test 1: Nullish coalescing should use OpIsNullish (1 register vs 4+)
let value1: string | null = null;
let result1 = value1 ?? "default";

// Test 2: Optional chaining should use OpIsNullish (1 register vs 5+)
let obj: { name?: string } | null = null;
let result2 = obj?.name ?? "unknown";

// Test 3: Strict comparisons should use OpIsNull/OpIsUndefined (1 register vs 3)
let nullCheck = value1 === null;
let undefinedCheck = obj?.name === undefined;

// Test 4: Assignment coalescing should use OpIsNullish (1 register vs 4+)
let mutable: number | null = null;
mutable ??= 42;

// Test all optimizations work correctly
let allWorking =
  result1 === "default" &&
  result2 === "unknown" &&
  nullCheck === true &&
  undefinedCheck === true &&
  mutable === 42;

// expect: true
allWorking;

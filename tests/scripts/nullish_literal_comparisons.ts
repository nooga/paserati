// Test strict equality/inequality with null and undefined literals
// This should use our efficient OpIsNull and OpIsUndefined opcodes

let x: number | null = 42;
let y: string | undefined = "hello";
let nullValue: any = null;
let undefinedValue: any = undefined;

// Test === with null literal (should use OpIsNull)
let test1 = x === null; // false
let test2 = nullValue === null; // true

// Test === with undefined literal (should use OpIsUndefined)
let test3 = y === undefined; // false
let test4 = undefinedValue === undefined; // true

// Test !== with null literal (should use OpIsNull + OpNot)
let test5 = x !== null; // true
let test6 = nullValue !== null; // false

// Test !== with undefined literal (should use OpIsUndefined + OpNot)
let test7 = y !== undefined; // true
let test8 = undefinedValue !== undefined; // false

// Mixed scenarios
let test9 = x !== null && y !== undefined; // true
let test10 = nullValue === null || undefinedValue === undefined; // true

// expect: false
test1;

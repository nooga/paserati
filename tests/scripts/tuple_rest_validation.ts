// Tuple rest element validation and type checking tests
// expect_compile_error: rest element in tuple type must be an array type

// Valid rest element cases - these should parse without errors
let validRest1: [string, ...number[]];
let validRest2: [...boolean[]];
let validRest3: [number, string, ...any[]];

// Test rest element to array assignment - should work when compatible
let arrayFromRest: number[];
// arrayFromRest = validRest2; // This would be invalid due to boolean vs number

// Test rest element to rest element assignment - should work when compatible
let compatible: [string, ...number[]] = validRest1;

// Invalid rest element cases - these should produce type errors

// Error 1: Rest element must be array type
let invalidRest1: [string, ...number];

// Error 2: Rest element must be array type
let invalidRest2: [...string];

// Error 3: Rest element must be array type
let invalidRest3: [boolean, ...any];

// Error 4: Invalid assignment - tuple with string rest to number array
let invalidAssignment: number[] = validRest1;

undefined;

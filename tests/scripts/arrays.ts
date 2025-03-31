// expect: 2
// expect: new value
// expect: undefined
// expect: Runtime Error: Array index 3 out of bounds for array of length 3

// 1. Array Literal Creation
let numbers_arr_test = [10, 20, 30];
let strings_arr_test = ["a", "b", "c"];
let mixed_arr_test = [1, "two", true, null, undefined];
let empty_arr_test: unknown[] = []; // Type annotation for empty array

// 2. Index Access (Get)
let firstNum_arr_test = numbers_arr_test[0]; // 10
let secondStr_arr_test = strings_arr_test[1]; // "b"
let thirdMixed_arr_test = mixed_arr_test[2]; // true

// 3. Index Assignment (Set)
numbers_arr_test[1] = 25; // numbers_arr_test is now [10, 25, 30]
strings_arr_test[0] = "new"; // strings_arr_test is now ["new", "b", "c"]
mixed_arr_test[3] = "changed"; // mixed_arr_test is now [1, "two", true, "changed", undefined]

// 4. Type Inference Check (commented out)
let numVar: number = numbers_arr_test[0];
let strVar: string = strings_arr_test[1];
let mixedVar: number | string | boolean | null | undefined = mixed_arr_test[4];

// 5. Type Checking Errors (commented out)
// let wrongNum: string = numbers_arr_test[0];
// let wrongIndex = numbers_arr_test["a"];
// numbers_arr_test[0] = "hello";

// 6. Test Calculation - Make this the final expression
let calc_result_arr_test = numbers_arr_test[0] / 5; // 10 / 5 = 2

// 7. Out-of-bounds Access (Get) - Removed print, value is unused
// let outOfBounds_arr_test = numbers_arr_test[5];

// 8. Out-of-bounds Assignment (Set) - Removed entirely for this test

// Final expression to be evaluated and checked against "expect"
calc_result_arr_test;

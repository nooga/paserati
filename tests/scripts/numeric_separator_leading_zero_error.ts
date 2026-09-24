// A numeric separator may only sit between two digits, and never after a
// leading zero.
// expect_compile_error: Numeric separators are not allowed after a leading 0

let x = 0_1;

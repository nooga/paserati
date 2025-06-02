// expect: 10.14

// Test unary plus (+) operator - converts values to numbers
let a = +"5"; // 5
let b = +true; // 1
let c = +false; // 0
let d = +"3.14"; // 3.14

// Test void operator - always returns undefined
let f = void 0; // undefined
let g = void 42; // undefined
let h = void "hello"; // undefined
let i = void true; // undefined

// Test that void always returns undefined
let voidTest =
  f === undefined && g === undefined && h === undefined && i === undefined
    ? 1
    : 0;

// Return the sum of numeric results plus void test result
// Should be 5 + 1 + 0 + 3.14 + 1 = 10.14
a + b + c + d + voidTest;

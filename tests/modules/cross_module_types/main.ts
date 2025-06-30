// Main module that imports from math module
import { Vector2D, add, magnitude, ZERO, UNIT_X } from './math';
import * as math from './math';

// Use imported type
let v1: Vector2D = { x: 3, y: 4 };
let v2: Vector2D = UNIT_X;

// Use imported function with type checking
let v3 = add(v1, v2);
console.log(magnitude(v3)); // expect: 5

// Use namespace import
let v4 = math.add(math.ZERO, math.UNIT_Y);
console.log(math.magnitude(v4)); // expect: 1

// Type error tests (should be caught by type checker)
// let badVector: Vector2D = { x: "not a number", y: 2 }; // expect_compile_error: Type 'string' is not assignable to type 'number'
// add(v1, "not a vector"); // expect_compile_error: Argument of type 'string' is not assignable to parameter of type 'Vector2D'

// Return the magnitude result
magnitude(v3);

// expect: 5
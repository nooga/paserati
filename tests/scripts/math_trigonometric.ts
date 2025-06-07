// Test trigonometric functions
let sin0 = Math.sin(0);
let sin90 = Math.sin(Math.PI / 2);
let cos0 = Math.cos(0);
let cos90 = Math.cos(Math.PI / 2);
let tan0 = Math.tan(0);

// Test with some tolerance for floating point precision
function isClose(a: number, b: number, tolerance: number = 0.0001): boolean {
  return Math.abs(a - b) < tolerance;
}

let trigCorrect =
  isClose(sin0, 0) &&
  isClose(sin90, 1) &&
  isClose(cos0, 1) &&
  isClose(cos90, 0) &&
  isClose(tan0, 0);

// expect: true
trigCorrect;

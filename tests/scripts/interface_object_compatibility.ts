// expect: {x: 30, y: 40}

interface Point {
  x: number;
  y: number;
}

// Test 1: Interface variable assigned to object literal
let someObj = { x: 30, y: 40 };
let c: Point = someObj;

c;

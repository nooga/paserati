// expect: {x: 100, y: 200}

interface Point {
  x: number;
  y: number;
}

interface Named {
  name: string;
}

// Test 1: Basic interface compatibility
let p1: Point = { x: 10, y: 20 };

// Test 2: Object with extra properties should be compatible (structural typing)
let objWithExtra = { x: 30, y: 40, z: 50 };
let p2: Point = objWithExtra;

// Test 3: Assignment between interface variables
let p3: Point = { x: 1, y: 2 };
let p4: Point = { x: 3, y: 4 };
p3 = p4;

// Test 4: Different interfaces with same structure should be compatible
interface Position {
  x: number;
  y: number;
}

let point: Point = { x: 100, y: 200 };
let position: Position = point;

point;

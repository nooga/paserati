// expect: 42

interface Point {
  x: number;
  y: number;
}

interface Named {
  name: string;
}

// Just test that interfaces are parsed correctly
let x = 42;
x;

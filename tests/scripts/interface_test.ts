// expect: 42

interface Point {
  x: number;
  y: number;
}

interface Named {
  name: string;
}

// Test basic object creation and typing
function createObject() {
  return { x: 10, y: 20, name: "test" };
}

let obj = createObject();
42;

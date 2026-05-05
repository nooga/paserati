// expect: 42

namespace A {
  export namespace B {
    export interface Point {
      x: number;
      y: number;
    }
    export const value = 42;
  }
}

// Multi-level qualified type in annotation
let p: A.B.Point = { x: 1, y: 2 };
A.B.value;

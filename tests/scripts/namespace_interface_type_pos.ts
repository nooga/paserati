// expect: 12

namespace N {
  export interface Pair {
    a: number;
    b: number;
  }
  export function sum(p: Pair): number {
    return p.a + p.b;
  }
}

const pair: N.Pair = { a: 5, b: 7 };
N.sum(pair);

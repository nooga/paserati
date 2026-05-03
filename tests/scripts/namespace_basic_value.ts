// expect: 7

namespace N {
  export const a = 3;
  export function f(x: number): number {
    return x + a;
  }
}

N.f(4);

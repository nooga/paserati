// expect: 7

namespace N {
  // function declarations hoist within the namespace body
  export const r = f(3);
  export function f(x: number): number {
    return x + 4;
  }
}

N.r;

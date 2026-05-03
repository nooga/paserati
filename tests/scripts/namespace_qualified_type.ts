// expect: 8

namespace M {
  export interface Box {
    val: number;
  }
  export function unwrap(b: Box): number {
    return b.val;
  }
}

function double(b: M.Box): number {
  return M.unwrap(b) * 2;
}

double({ val: 4 });

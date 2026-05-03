// expect: 30

namespace N {
  export const x = 10;
}

namespace N {
  export const y = 20;
}

N.x + N.y;

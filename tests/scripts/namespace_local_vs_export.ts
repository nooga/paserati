// expect: 5

namespace N {
  const secret = 99;
  export const visible = 5;
  export function useSecret(): number {
    return secret;
  }
}

// secret is not accessible from outside
// N.secret would be a type error
N.visible;

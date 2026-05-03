// expect: 99

namespace Outer {
  export namespace Inner {
    export const x = 99;
  }
}

Outer.Inner.x;

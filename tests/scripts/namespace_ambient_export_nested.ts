// Ambient exports inside a namespace emit nothing; `export declare namespace M`
// inside `namespace M` used to recurse until the compiler ran out of registers.
namespace M {
  export declare var x: number;
  export declare class C { }
  export declare namespace M { }
  export const y = 2;
}
Object.keys(M).join(",");

// expect: y

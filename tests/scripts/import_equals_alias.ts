// import x = N.M aliases an entity: its value at runtime, its type in types.
// expect: 7
namespace N { export const v = 5; export namespace M { export const w = 2; export type T = number; } }
import x = N.v;
import y = N.M;
const t: y.T = x;
t + y.w;

// An alias resolved before X has a type keeps (typeof X)[K] deferred; uses of
// the alias index X's type once it is known.
interface Z<O> { _output: O; }
declare const S: Z<{ n: number }>;
type T = (typeof S)["_output"];
let t: T;
t = { n: "x" };

// expect_compile_error: Type 'string' is not assignable to type 'number'.

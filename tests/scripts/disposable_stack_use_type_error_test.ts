// expect_compile_error: not assignable

// DisposableStack.prototype.use<T> infers T from its argument and returns
// that same T - so binding the result to an incompatible declared type must
// be a compile error, not silently widen to `any`.

let stack = new DisposableStack();
let n: number = stack.use("not a number");
n;

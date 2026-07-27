// expect_compile_error: Variable 'x' is used before being assigned.
// TS2454: reading an annotated, uninitialized variable before any assignment.

function f(): number {
    let x: number;
    return x;
}

f();

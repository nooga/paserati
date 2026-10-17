// expect_compile_error: Variable 'i' is used before being assigned.
// A write inside a closure does not reach the enclosing flow, as in tsc.
function viaClosure(): number {
    let i: number;
    const set = () => { i = 9; };
    set();
    return i;
}
viaClosure();

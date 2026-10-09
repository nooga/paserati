// expect_compile_error: No overload matches this call
function f(a: string, c: string): void;
function f(a: number, b: number): void;
function f(a: any, b?: any) {}
f(true, 1);

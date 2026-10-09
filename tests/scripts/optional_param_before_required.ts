// expect_compile_error: A required parameter cannot follow an optional parameter
function f(a?: string, b: number): void {}

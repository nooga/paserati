// expect_compile_error: decorator must be a callable expression
const notAFunction: number = 42;

@notAFunction
class MyClass {}

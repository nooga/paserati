// expect_compile_error: Expected 1 arguments, but got 2.

// Detect too many arguments passed to a plain (non-variadic) function call
// (TS2554), while making sure legal excess-argument forms stay silent.

// Legal: a rest parameter accepts any number of arguments.
function withRest(...xs: number[]): number {
	return xs.length;
}
withRest(1, 2, 3, 4);

// Legal: an optional parameter raises the maximum accepted count.
function withOptional(a: number, b?: number): number {
	return a + (b ?? 0);
}
withOptional(1);
withOptional(1, 2);

// Illegal: plain function called with more arguments than declared.
function g(a: number): void {}
g(1, 2);

"error test";

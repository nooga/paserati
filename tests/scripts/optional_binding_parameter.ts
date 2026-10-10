// An optional binding pattern is fine in a signature, not in an implementation.
// expect_compile_error: A binding pattern parameter cannot be optional in an implementation signature.
declare function sig({ a }?: { a: number }): void;
function impl({ a }?: { a: number }) {}

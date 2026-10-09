// expect_compile_error: does not satisfy the constraint
function f<T extends { id: string }>(x: T): string { return x.id; }
f<{ name: string }>({ name: "a" });

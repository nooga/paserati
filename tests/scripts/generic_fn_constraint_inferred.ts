// expect_compile_error: Object literal may only specify known properties
function f<T extends { id: string }>(x: T): string { return x.id; }
f({ name: "a" });

// A type annotation on an array destructuring declaration governs the binding
// types even when the initializer isn't an array literal (e.g. an `any`-typed
// expression), matching how object destructuring already resolves bindings
// from the annotation rather than the initializer's own inferred type.
let source: any = [1, "s"];
let [a, b]: [number, string] = source;
let s: string = a; // 'a' must be typed 'number', not 'any' -- this should fail
s;
// expect_compile_error: cannot assign type

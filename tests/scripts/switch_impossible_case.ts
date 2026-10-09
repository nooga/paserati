// expect_compile_error: Type '"c"' is not comparable to type
type K = "a" | "b";
function f(k: K) { switch (k) { case "c": break; } }

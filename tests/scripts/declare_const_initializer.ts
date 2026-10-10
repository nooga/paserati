// An ambient constant with a literal initializer has the literal's type.
// expect_compile_error: is not assignable to type '"1.1"'.
declare const VERSION = "1.0";
const ok: "1.0" = VERSION;
const bad: "1.1" = VERSION;

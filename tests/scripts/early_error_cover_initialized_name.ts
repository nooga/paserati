// expect_compile_error: Invalid shorthand property initializer
// `{a = 1}` is only valid as a destructuring pattern; as an object literal it
// is an early error.
const o = ({ a = 1 });

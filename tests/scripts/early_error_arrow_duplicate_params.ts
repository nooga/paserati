// expect_compile_error: Duplicate identifier 'a'
// Arrow parameters may never repeat a name, even in sloppy mode.
const f = (a, a) => a;

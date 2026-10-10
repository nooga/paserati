// expect_compile_error: Cannot find module './import_type_nope.ts' or its corresponding type declarations.
const x: import("./import_type_nope.ts").X = 1;

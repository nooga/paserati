// An optional chain cannot be the tag of a tagged template.
// expect_compile_error: Tagged template cannot be used in optional chain
// no-typecheck

const o: any = { fn() {} };
o?.fn`x`;

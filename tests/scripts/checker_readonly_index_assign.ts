// expect_compile_error: Index signature in type 'readonly number[]' only permits reading
const xs: readonly number[] = [1, 2];
xs[0] = 3;

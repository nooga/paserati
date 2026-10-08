// expect_compile_error: Property 'push' does not exist on type 'readonly number[]'
const xs: ReadonlyArray<number> = [1];
xs.push(2);

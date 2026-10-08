// expect_compile_error: The type 'readonly string[]' is 'readonly' and cannot be assigned to the mutable type 'string[]'
const ro: ReadonlyArray<string> = ["a"];
const m: string[] = ro;

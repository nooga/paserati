import * as z from "./star_import_types_helper.ts";
type S = { _o: { n: number } };
const bad: z.infer<S> = { n: "x" };

// expect_compile_error: Type 'string' is not assignable to type 'number'.

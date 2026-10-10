import { z } from "./reexport_namespace_helper.ts";
const p: z.Pair<string> = z.make(2);

// expect_compile_error: is not assignable to type

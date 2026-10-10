import * as z from "./zodlike_classes_helper.ts";
function check(s: z.Str): void {
  const bad: number = s.opt()._o;
}

// expect_compile_error: Type 'string | undefined' is not assignable to type 'number'.

import { x, f, C, util } from "./export_declare_helper.ts";
function g(c: C): void {
  const a: number = x + f() + c.m();
  const b: util.Keep<number> = a;
  const bad: string = c.m();
}

// expect_compile_error: Type 'number' is not assignable to type 'string'.

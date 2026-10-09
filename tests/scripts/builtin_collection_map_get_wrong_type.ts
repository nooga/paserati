// expect_compile_error: Type 'number' is not assignable
const m = new Map<string, number>(); const s: string = m.get("a")!;

// expect_compile_error: not assignable to parameter of type 'string'
declare function use(x: string): void;
declare const flag: boolean;
function f(id: string | undefined) { if (!id) return; while (flag) { id = undefined; } use(id); }

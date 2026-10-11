// `declare global { ... }` in a module declares globals.
// expect_compile_error: Type 'number' is not assignable to type 'string'.
declare global {
  var configured: number;
  interface Settings { level: 1 }
}
export {};
const s: Settings = { level: 1 };
const x: string = configured;

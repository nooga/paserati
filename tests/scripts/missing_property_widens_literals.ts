// expect_compile_error: is missing in type '{ name: string }' but required in type 'D'
interface D { name: string; size: number }
const d: D = { name: "x" };

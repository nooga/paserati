// import("x") in a type names the module's namespace: qualified types
// (generic, nested namespace), and with typeof its values. The module is
// loaded on demand, no import declaration needed.
// expect: x1abc2
const a: import("./import_type_helper.ts").Box<string> = { v: "x" };
const b: import("./import_type_helper.ts").NS.Inner = 1;
const c: typeof import("./import_type_helper.ts").k = { n: 2 };
type M = typeof import("./import_type_helper.ts");
const d: M["k"]["n"] = 3;
const e: ReturnType<typeof import("./import_type_helper.ts").default> = 4;
const id: import("./import_type_helper.ts", { with: { "resolution-mode": "import" } }).Id = "abc";
`${a.v}${b}${id}${c.n}`;

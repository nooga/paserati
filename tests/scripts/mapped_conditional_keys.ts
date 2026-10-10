// zod's requiredKeys/optionalKeys: a conditional on a generic T[k] (on the
// extends side) is deferred until T is known, and `{...}[keyof T]` is
// computed once T is substituted.
type S0 = { n: number; s: string | undefined };
type optionalKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? k : never }[keyof T];
type requiredKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? never : k }[keyof T];
const r: requiredKeys<S0> = "n";
const o: optionalKeys<S0> = "s";
const bad: requiredKeys<S0> = "s";

// expect_compile_error: Type '"s"' is not assignable to type '"n"'.

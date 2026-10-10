type S0 = { n: number; s: string | undefined };
type optionalKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? k : never }[keyof T];
type requiredKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? never : k }[keyof T];
const r: requiredKeys<S0> = "n";
const o: optionalKeys<S0> = "s";
r + o;

// expect: ns

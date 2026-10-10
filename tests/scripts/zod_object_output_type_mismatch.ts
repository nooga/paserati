declare class ZodType<Output = any, Def = any, Input = Output> { readonly _output: Output; readonly _input: Input; }
declare class ZodNumber extends ZodType<number> {}
declare class ZodOptional<T extends ZodType> extends ZodType<T["_output"] | undefined> {}
type ZodRawShape = { [k: string]: ZodType };
type ZodTypeAny = ZodType<any, any, any>;
type optionalKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? k : never }[keyof T];
type requiredKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? never : k }[keyof T];
type addQuestionMarks<T extends object> = { [K in requiredKeys<T>]: T[K] } & { [K in optionalKeys<T>]?: T[K] } & { [k in keyof T]?: unknown };
type identity<T> = T;
type flatten<T> = identity<{ [k in keyof T]: T[k] }>;
type baseObjectOutputType<Shape extends ZodRawShape> = { [k in keyof Shape]: Shape[k]["_output"] };
type CatchallOutput<T extends ZodType> = ZodType extends T ? unknown : { [k: string]: T["_output"] };
type UnknownKeysParam = "passthrough" | "strict" | "strip";
type PassthroughType<T extends UnknownKeysParam> = T extends "passthrough" ? { [k: string]: unknown } : unknown;
type objectOutputType<Shape extends ZodRawShape, Catchall extends ZodTypeAny, UnknownKeys extends UnknownKeysParam = UnknownKeysParam> = flatten<addQuestionMarks<baseObjectOutputType<Shape>>> & CatchallOutput<Catchall> & PassthroughType<UnknownKeys>;
type Out = objectOutputType<{ n: ZodNumber; o: ZodOptional<ZodNumber> }, ZodTypeAny, "strip">;
const bad: Out = { n: "x" };

// expect_compile_error: Type 'string' is not assignable to type 'number'.

// zod_object_output_type.ts with every alias first, in reverse dependency
// order, and the classes last (zod's own files declare the aliases before
// the classes they name): resolution must not depend on declaration order.
type Out = objectOutputType<{ n: ZodNumber; o: ZodOptional<ZodNumber> }, ZodTypeAny, "strip">;
type objectOutputType<Shape extends ZodRawShape, Catchall extends ZodTypeAny, UnknownKeys extends UnknownKeysParam = UnknownKeysParam> = flatten<addQuestionMarks<baseObjectOutputType<Shape>>> & CatchallOutput<Catchall> & PassthroughType<UnknownKeys>;
type PassthroughType<T extends UnknownKeysParam> = T extends "passthrough" ? { [k: string]: unknown } : unknown;
type UnknownKeysParam = "passthrough" | "strict" | "strip";
type CatchallOutput<T extends ZodType> = ZodType extends T ? unknown : { [k: string]: T["_output"] };
type baseObjectOutputType<Shape extends ZodRawShape> = { [k in keyof Shape]: Shape[k]["_output"] };
type flatten<T> = identity<{ [k in keyof T]: T[k] }>;
type identity<T> = T;
type addQuestionMarks<T extends object> = { [K in requiredKeys<T>]: T[K] } & { [K in optionalKeys<T>]?: T[K] } & { [k in keyof T]?: unknown };
type requiredKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? never : k }[keyof T];
type optionalKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? k : never }[keyof T];
type ZodTypeAny = ZodType<any, any, any>;
type ZodRawShape = { [k: string]: ZodType };
declare class ZodType<Output = any, Def = any, Input = Output> { readonly _output: Output; readonly _input: Input; }
declare class ZodNumber extends ZodType<number> {}
declare class ZodOptional<T extends ZodType> extends ZodType<T["_output"] | undefined> {}
const a: Out = { n: 1 };
const b: Out = { n: 1, o: 2 };
const c: number = a.n;
c + (b.o ?? 0);


// expect: 3

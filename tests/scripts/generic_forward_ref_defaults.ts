// A generic declared later with defaulted type parameters can be named bare
// (zod's `unionErrors: ZodError[]` before `class ZodError<T = any>`), and a
// self-referencing generic class instantiates without recursing forever.
interface Holder { items: Box[]; }
declare class Box<T = number> { value: T; again(): Box<this>; }
function first(h: Holder): number { return h.items[0].value; }
function bad(h: Holder): string { return h.items[0].value; }

// expect_compile_error: Type 'number' is not assignable to type 'string'.

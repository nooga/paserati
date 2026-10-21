// expect: hi
// An interface extending one whose own base is declared further down gets
// the members of the whole chain.
interface ZString extends ZType<string> { min(): ZString }
interface ZType<O = unknown> extends Base<O> { parse(x: unknown): O }
interface Base<O = unknown> { _zod: { output: O } }
const z = { _zod: { output: "hi" } } as ZString;
const out: string = z._zod.output;
out;

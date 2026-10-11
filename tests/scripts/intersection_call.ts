// expect: 3
// An intersection is callable with its members' call signatures (vitest's
// `test` is `ChainableTestAPI & ExtendedAPI & Hooks & {...}`).
interface Callable<C = object> {
  <E extends C>(name: string, fn?: (x: E) => void): number;
}
type API = { each: number } & Callable<object>;
const impl = ((name: string) => name.length) as any;
const test: API = impl;
test("abc");

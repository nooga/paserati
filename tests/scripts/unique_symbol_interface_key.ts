// expect: 3
declare const hook: unique symbol;
interface Emitter<T> {
  [hook]?<K>(error: Error, event: K): void;
  on(name: string): number;
}
interface Named extends Emitter<{ a: [] }> {
  toIterable<N extends string>(name: N): number;
}
const e: Named = { on: () => 1, toIterable: () => 2 };
e.on("x") + e.toIterable("y");

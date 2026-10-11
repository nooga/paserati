// An optional method with a computed key and type parameters, as
// @types/node's EventEmitter declares.
// expect: 1
const key: unique symbol = Symbol("k");
declare class Handler {
  [key]?<K>(k: K): void;
  plain?(): void;
}
declare const h: Handler;
1;

// expect: function
// lib.es5's PromiseLike<T>: a union with Promise<T> has then, and await
// unwraps it.
class Box<T> {
  readonly promise: Promise<T> | PromiseLike<T>;
  constructor(p: Promise<T> | PromiseLike<T>) { this.promise = p; }
  map<U>(f: (v: T) => U): PromiseLike<U> { return this.promise.then(f); }
}
async function run(): Promise<number> {
  const like: PromiseLike<number> = Promise.resolve(2);
  const n = await new Box(like).map((v) => v + 1);
  return n;
}
run();
typeof new Box(Promise.resolve(1)).map((v) => v + 1).then;

// expect: 3
// InstanceType of a generic class's constructor reads the class with its
// type parameters at their constraints (Resp<unknown>), and a callback
// parameter typed `InstanceType<S> & {...}` with S not inferred reads S's
// constraint, inherited members included (@types/node's RequestListener).
declare class Outgoing {
  end(): void;
}
declare class Resp<Q = number> extends Outgoing {
  q: Q;
  writeHead(code: number): this;
}
declare class Strict<Q extends string> {
  q: Q;
}

type Erased = InstanceType<typeof Resp>;
const erased: Erased = { q: "anything" } as Erased;
const strictQ: InstanceType<typeof Strict>["q"] = "s";

function listen<S extends typeof Resp = typeof Resp>(
  listener: (res: InstanceType<S> & { tag: string }) => number,
): number {
  return 3;
}

listen((res) => {
  res.end();
  res.writeHead(200);
  return res.tag.length;
});

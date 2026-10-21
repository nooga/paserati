// expect: ok
// A namespace merged with a class adds its exports to the class: the value
// S.R and the types S.R and S.Opts.
class S {
  pipe(): string {
    return "pipe";
  }
}
namespace S {
  export class R extends S {
    read(): string {
      return "ok";
    }
  }
}
namespace S {
  export interface Opts {
    a: number;
  }
}
const r: S.R = new S.R();
const o: S.Opts = { a: 1 };
r.pipe() === "pipe" && o.a === 1 ? r.read() : "bad";

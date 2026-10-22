// expect: def
// A variable holding a generic interface instance keeps its literal members
// (only fresh object literals widen), and a generic method of a call's
// instantiated result reads the call's solution.
interface Opts { ef?: (x: number) => { shape: 1 } }
interface Plain<R> { cfg: R }
interface Builder<TE> {
  query<$O>(r: () => $O): Plain<{ out: $O; es: TE }>;
}
interface Root<O extends Opts, R extends { es: unknown } = { es: undefined extends O["ef"] ? "def" : 0 }> {
  cfg: R;
  proc: Builder<R["es"]>;
}
class Init {
  create<O extends Opts>(o?: O): Root<O> {
    return {
      cfg: { es: "def" },
      proc: { query: (r: any) => ({ cfg: { out: r(), es: "def" } }) },
    } as any;
  }
}
const p: Plain<{ es: "def" }> = { cfg: { es: "def" } };
const copy = p;
const lit: "def" = copy.cfg.es;
const t = new Init().create();
const es: "def" = t.cfg.es;
const fromMethod: "def" = t.proc.query(() => 1).cfg.es;
fromMethod;

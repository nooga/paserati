// expect: 5
// A generic indexed access relates through its base constraint, and one
// that constraint makes any is called untyped (TRoot['ctx'] of { ctx: any }).
interface Shape { code: number }
interface RootTypes { ctx: any; errorShape: Shape }
interface RootConfig<T extends RootTypes> { $types: T }
interface Router<T extends RootTypes> { _def: { _config: RootConfig<T> } }

function code<R extends Router<RootTypes>>(e: R["_def"]["_config"]["$types"]["errorShape"]): number {
  const s: Shape = e;
  return s.code;
}

function call<T extends RootTypes>(ctx: T["ctx"]): number {
  return ctx();
}

code({ code: 2 }) + call(() => 3);

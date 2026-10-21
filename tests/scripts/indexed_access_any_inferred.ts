// expect: 1
// TRoot inferred as any makes TRoot['ctx'] any too.
interface Types { ctx: object }
interface Config<T extends Types> { $types: T }
function shape<TRoot extends Types>(o: { config: Config<TRoot>; ctx: TRoot["ctx"] | undefined }): number {
  return 1;
}
declare let anyConfig: Config<any>;
anyConfig = { $types: { ctx: {} } };
function f<C>(ctx: C | undefined) {
  return shape({ config: anyConfig, ctx });
}
f(undefined);

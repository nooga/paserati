// expect: 3
// R infers from an indexed access through R to an indexed access through the
// caller's R, alone or beside fixed union members (tRPC's
// inferRouterContext<TRouter> | undefined).
interface Types { ctx: any }
interface Config<T extends Types> { $types: T }
interface Def<T extends Types> { _config: Config<T> }
interface Router<T extends Types> { _def: Def<T> }
type AnyRouter = Router<any>;
type inferCtx<R extends AnyRouter> = R['_def']['_config']['$types']['ctx'];
function init<R extends AnyRouter>(o: { ctx: inferCtx<R> | undefined }): number { return 1; }
function init2<R extends AnyRouter>(o: { ctx: inferCtx<R> }): number { return 1; }
function init3<R extends AnyRouter>(ctx: inferCtx<R> | undefined): number { return 1; }
function outer<R extends AnyRouter>(ctx: inferCtx<R> | undefined, c2: inferCtx<R>): number {
  return init({ ctx }) + init2({ ctx: c2 }) + init3(ctx);
}
outer(undefined, 1);

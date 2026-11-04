// expect_compile_error: Type '{ query:
// A conditional whose check type indexes an `infer` variable not inferred
// yet (`$Value['_def']['type']`) waits for it instead of picking a branch.
type ProcedureType = 'query' | 'mutation';
type Def = { input: any; output: any };
type Resolver<TDef extends Def> = (input: TDef['input']) => Promise<TDef['output']>;
type DecorateProcedure<TType extends ProcedureType, TDef extends Def> = TType extends 'query'
  ? { query: Resolver<TDef> }
  : TType extends 'mutation'
    ? { mutate: Resolver<TDef> }
    : never;
interface Proc<TType extends ProcedureType> { _def: { type: TType } }
type Rec<TRecord> = {
  [TKey in keyof TRecord]: TRecord[TKey] extends infer $Value
    ? $Value extends Proc<any>
      ? DecorateProcedure<$Value['_def']['type'], { input: 1; output: 2 }>
      : never
    : never;
};
declare const r: Rec<{ hello: Proc<'query'> }>;
const x: number = r.hello;

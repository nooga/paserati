// expect: query
// A heritage type argument checked against `(typeof xs)[number]`, where the
// const xs has no type yet when interfaces are declared, waits for it.
export const procedureTypes = ['query', 'mutation', 'subscription'] as const;
export type ProcedureType = (typeof procedureTypes)[number];
interface Procedure<TType extends ProcedureType> { _type: TType }
interface QueryProcedure extends Procedure<'query'> {}
const q: QueryProcedure = { _type: 'query' };
const t: 'query' = q._type;
t;

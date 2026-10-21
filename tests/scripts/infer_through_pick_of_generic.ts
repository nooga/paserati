// expect: 3
// Pick<Router<TRecord>, '_def'> infers TRecord from the picked members.
interface Def<TRecord> { record: TRecord }
interface Router<TRecord> { _def: Def<TRecord>; extra: number }
type RouterRecord = { [key: string]: unknown };
function direct<TRecord extends RouterRecord>(r: Pick<Router<TRecord>, '_def'>): TRecord {
  return r._def.record;
}
const merged: Router<{ foo: string }> = { _def: { record: { foo: "abc" } }, extra: 0 };
direct(merged).foo.length;

// Throwing null/undefined is a real exception in generators and async
// functions too: nullish values are not "no exception" (#566).
// expect: gen:null,gen:undefined,async:null,async:undefined,agen:null
const log: string[] = [];
function* g(v: any) { throw v; }
for (const v of [null, undefined]) {
  try { g(v).next(); } catch (e) { log.push("gen:" + e); }
}
async function f(v: any) { await 0; throw v; }
for (const v of [null, undefined]) {
  try { await f(v); } catch (e) { log.push("async:" + e); }
}
async function* ag() { throw null; }
try { await ag().next(); } catch (e) { log.push("agen:" + e); }
log.join(",");

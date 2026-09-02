// expect: 10|105|A-value|B-value
// Follow-up to #183 for dynamic import(): a function exported from a/dyn.ts
// and called from here must resolve its import("./_helper.ts") against a/,
// not against this file's directory (which has no _helper.ts) or against
// whichever module happened to run last.
import { dynA, dynOnlyA } from "./import_same_basename/a/dyn.ts";
import { dynB, dynOnlyB } from "./import_same_basename/b/dyn.ts";

const a = await dynA(5);
const b = await dynB(5);
const onlyA = await dynOnlyA();
const onlyB = await dynOnlyB();
`${a}|${b}|${onlyA}|${onlyB}`;

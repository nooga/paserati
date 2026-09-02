// expect: A-value|B-value|10|105
// Regression test for #183: two files in different directories imported with
// the identical relative specifier ("./_helper.ts") must each load their own
// file. Before the fix, whichever resolved first was silently reused for both,
// so useB(5) returned 10 and onlyInB was undefined.
import { useA, a } from "./import_same_basename/a/user.ts";
import { useB, b } from "./import_same_basename/b/user.ts";

`${a()}|${b()}|${useA(5)}|${useB(5)}`;

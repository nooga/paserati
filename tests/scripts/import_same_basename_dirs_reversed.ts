// expect: B-value|A-value|105|10
// Same as import_same_basename_dirs.ts with the import order flipped; before
// the fix this flipped which side broke.
import { useB, b } from "./import_same_basename/b/user.ts";
import { useA, a } from "./import_same_basename/a/user.ts";

`${b()}|${a()}|${useB(5)}|${useA(5)}`;

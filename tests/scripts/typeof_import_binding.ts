// expect: function,number,function,function,number,undefined,object,function
// skip-typecheck
// Regression test for #100: typeof on an ESM import binding must resolve
// the real import, not fall through to the "identifier doesn't exist"
// OpTypeofIdentifier path (which always reports "undefined" under
// skip-typecheck, since imports never make it into the symbol table).
import def, { foo, q, Bar, foo as f } from "./typeof_import_helper.ts";
import * as ns from "./typeof_import_helper.ts";

function closureOverImport() {
  return typeof foo;
}

[
  typeof foo,
  typeof q,
  typeof Bar,
  typeof f,
  typeof def,
  typeof notDefined,
  typeof ns,
  closureOverImport(),
].join(",");

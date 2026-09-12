// expect: true
// paserati#432: `export { x as undefined }` failed to parse at all -
// isExportSpecifierName's hand-maintained allowlist of keyword tokens
// permitted as an export specifier name was missing lexer.UNDEFINED. This
// is a real-world shape: `undefined` is not a reserved word in ECMAScript
// (the export-specifier grammar uses the broader IdentifierName production,
// not Identifier), and real zod@3.25.76's own v3/types.js exports exactly
// this way (`export { undefinedType as undefined, ... }` - `z.undefined()`
// is one of zod's real schema builders). Real Node parses and runs this
// without complaint; paserati failed the whole module's parse, so nothing
// from the file - not just this one export - ever loaded.
import * as ns from "./export_named_as_undefined_helper.ts";

typeof ns.undefined === "function" && ns.undefined() === "I am undefinedType";

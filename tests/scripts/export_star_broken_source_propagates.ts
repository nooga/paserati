// expect_compile_error: Failed to load module './export_star_broken_source_helper_lib.ts' for re-export
// paserati#433: "export * from a module that fails to load/parse" was
// silently dropped instead of propagating the error.
//
// Real Node fails the whole importing module synchronously and eagerly when
// a re-exported ("export * from") module fails to parse - it does not just
// quietly omit that module's names from the merged namespace. Paserati used
// to do exactly that: compileExportAllDeclaration's bare "export *" path
// (unlike "export * as ns from" or a plain named/namespace import) never
// called emitEvalModule/emitGetModuleExport per name when the source
// module's export names came back empty, so a failed dependency had no
// bytecode path left to surface the failure through at all - not even at
// runtime.
//
// export_star_broken_source_helper_mid.ts does `export * from
// "./export_star_broken_source_helper_lib.ts"`, and that lib module
// deliberately fails to parse. Importing the barrel must now fail the
// import at compile time instead of silently producing a namespace missing
// just that module's names.
import * as ns from "./export_star_broken_source_helper_mid.ts";

ns.other;

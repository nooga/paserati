// expect_compile_error: exported name 'NS' not found in current scope
// paserati#163: re-exporting a *namespace* import (import * as NS; export
// { NS };) has no established resolution path (there's no real "*" export
// name to look up in the source module), so this must keep erroring
// loudly rather than silently compiling to an undefined re-export.
import { NS } from "./reexport_of_namespace_import_index.ts";

NS;

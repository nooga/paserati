// z in a value position is the module namespace object with its export
// types, not any, even though z is also a namespace on the type side.
import * as z from "./star_import_types_helper.ts";
const s: string = z.marker;

// expect_compile_error: Type 'number' is not assignable to type 'string'.

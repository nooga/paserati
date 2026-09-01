// expect: diff
// paserati#163: re-exporting a default import (barrel-file pattern),
// consumed here through a namespace import.
import * as NS from "./reexport_of_default_import_index.ts";

const d = new NS.Diff();
d.kind();

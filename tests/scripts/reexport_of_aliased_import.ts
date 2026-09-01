// expect: 99
// paserati#163: re-exporting an aliased import (import { Foo as Bar };
// export { Bar };) must resolve through the import's *source* name
// ("Foo"), not the local alias ("Bar"), when looking it up in base.ts.
import { Bar } from "./reexport_of_aliased_import_index.ts";

Bar;

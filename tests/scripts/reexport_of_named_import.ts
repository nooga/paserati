// expect: 42
// paserati#163: export { Foo } where Foo is an imported (not locally
// declared) binding must resolve through the import's source module.
import { Foo } from "./reexport_of_named_import_index.ts";

Foo;

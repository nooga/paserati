// skip-typecheck
// expect: 42
// paserati#440: `import { foo as undefined } from "./mod"` is the real-world
// shape that surfaces this bug (see paserati#432/#433 - zod's v3/types.js
// exports exactly this way). Once the specifier parses, the resulting local
// binding named `undefined` was silently inert: every read of `undefined`
// still resolved to the literal undefined value instead of the imported one.
import { foo as undefined } from "./import_as_undefined_helper.ts";

console.log(typeof undefined);
undefined;

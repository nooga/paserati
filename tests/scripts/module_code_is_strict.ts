// expect: 3
// skip-typecheck
// Module code is strict, with export default function as a declaration:
// the parenthesised call below is a separate statement.
export default function () { return 1; }
(function () { return this === undefined; })() ? 3 : 0;
import "./module_live_bindings/empty.ts";

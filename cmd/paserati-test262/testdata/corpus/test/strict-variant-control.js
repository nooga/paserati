/*---
description: Control. No strictness flags; passes in both variants.
---*/
function f() { return this; }
assert(f() === undefined || typeof f() === "object");

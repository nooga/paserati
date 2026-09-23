/*---
description: Audit fixture. No strictness flags; throws only when this is undefined, so the required strict variant fails.
---*/
function f() { return this; }
if (f() === undefined) throw new Test262Error("strict variant");

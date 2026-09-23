/*---
description: noStrict runs only the sloppy variant.
flags: [noStrict]
---*/
with ({}) {}
function f() { return this; }
assert(f() !== undefined);

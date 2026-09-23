/*---
description: onlyStrict runs only the strict variant.
flags: [onlyStrict]
---*/
function f() { return this; }
assert.sameValue(f(), undefined);

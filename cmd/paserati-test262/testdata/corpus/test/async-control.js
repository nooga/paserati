/*---
description: Control. Async test that completes.
flags: [async]
---*/
Promise.resolve(1).then(function (v) { assert.sameValue(v, 1); }).then($DONE, $DONE);

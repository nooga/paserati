/*---
description: Audit fixture. Async test that reports failure through $DONE.
flags: [async]
---*/
Promise.resolve().then(function () { $DONE(new Error("FAIL")); });

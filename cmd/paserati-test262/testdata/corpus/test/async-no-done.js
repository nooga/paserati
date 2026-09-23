/*---
description: Audit fixture. Async test that never calls $DONE.
flags: [async]
---*/
Promise.resolve().then(function () {});

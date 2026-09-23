/*---
description: Async failure wins over an earlier completion signal.
flags: [async]
---*/
$DONE();
Promise.resolve().then(function () { $DONE(new Test262Error("late failure")); });

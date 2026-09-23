/*---
description: A promise job that queues itself forever; cancellation must stop the drain.
flags: [noStrict]
---*/
function again() { Promise.resolve().then(again); }
again();

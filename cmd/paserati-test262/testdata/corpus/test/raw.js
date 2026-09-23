/*---
description: raw tests get no harness.
flags: [raw]
---*/
if (typeof assert !== "undefined") throw new Error("harness was included");

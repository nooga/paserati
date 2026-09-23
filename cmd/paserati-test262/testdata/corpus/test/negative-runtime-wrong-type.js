/*---
description: Audit fixture. Declares a runtime TypeError but throws RangeError.
negative:
  phase: runtime
  type: TypeError
---*/
throw new RangeError("wrong type");

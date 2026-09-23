/*---
description: Audit fixture. Declares a parse SyntaxError but parses and throws one at runtime.
negative:
  phase: parse
  type: SyntaxError
---*/
throw new SyntaxError("thrown at runtime");

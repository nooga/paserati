// Regression test for issue #480: trimLeft/trimRight are ECMA-262 Annex B.2.3
// aliases of trimStart/trimEnd - same function object, same behavior.
// expect: true
// no-typecheck

"  hi  ".trimLeft() === "hi  " &&
  "  hi  ".trimRight() === "  hi" &&
  String.prototype.trimLeft === String.prototype.trimStart &&
  String.prototype.trimRight === String.prototype.trimEnd;

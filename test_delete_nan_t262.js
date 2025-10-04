/*---
flags: [noStrict]
---*/
var d = delete NaN;
if (d !== false) {
  throw new Error("Expected delete NaN to return false, got " + d);
}
if (typeof NaN === "undefined") {
  throw new Error("NaN should still exist after delete");
}
console.log("SUCCESS");

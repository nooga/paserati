// Minimal stand-in for test262's harness/sta.js, for the runner's own tests.
function Test262Error(message) { this.message = message || ""; }
Test262Error.prototype.toString = function () { return "Test262Error: " + this.message; };
Test262Error.thrower = function (message) { throw new Test262Error(message); };
function $DONOTEVALUATE() { throw "Test262: This statement should not be evaluated."; }

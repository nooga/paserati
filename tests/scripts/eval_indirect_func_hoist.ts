// expect: 42
// no-typecheck
// Test indirect eval function declarations hoist to global scope
// Function declared in indirect eval should be accessible via globalThis

(0,eval)("function evalFunc() { return 42; }");
globalThis.evalFunc();

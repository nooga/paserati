// expect: true
// no-typecheck
// Test indirect eval var declarations are accessible via globalThis
// Per ECMAScript, var declarations in non-strict indirect eval become global properties

(0,eval)("var globalVarTest = 42;");
globalThis.globalVarTest === 42;

// expect: 123
// no-typecheck
// Test indirect eval var declarations hoist to global scope
// Var declared in indirect eval should be accessible via globalThis
// (Direct access would fail type checking since the var is only known at runtime)

(0,eval)("var evalVar = 123;");
globalThis.evalVar;

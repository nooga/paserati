// expect: 42
// Test yield as parameter name in non-generator function (non-strict mode)
var obj = { method(yield) { return yield; } };
obj.method(42);

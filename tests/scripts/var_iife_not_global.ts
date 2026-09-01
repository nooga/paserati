// expect_compile_error: Cannot find name 'x'
// `var` is function-scoped, so a var inside an IIFE must not be visible outside
// it. This used to be caught only at runtime ("ReferenceError: x is not
// defined") because the checker gave a function body a *block*-scoped
// environment: GetFunctionScope, where every var binding lands, walked straight
// past the function body up to the enclosing function or global scope. The
// checker now rejects it, which is what this test's original comment asked for.

(function () {
  var x = 42;
})();

x; // Should be compile error - x not defined

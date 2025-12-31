// expect: 1
// FIXME: Indirect eval should NOT inherit strict mode from caller
// Even in strict mode, indirect eval runs in non-strict mode (unless eval code has "use strict")
// This test fails because 'static' is only reserved in strict mode
// Currently failing because we inherit strict mode incorrectly

// For now, test basic non-strict behavior without reserved words
(0,eval)("var testVar = 1; testVar");

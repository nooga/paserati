// expect_compile_error: Cannot find name 'cosnole'. Did you mean 'console'?
// TS2552: an unresolved name close enough (by weighted Levenshtein distance)
// to a name visible in scope gets a spelling suggestion instead of the plain
// TS2304 "Cannot find name" diagnostic.

cosnole.log("hello");

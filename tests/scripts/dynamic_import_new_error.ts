// expect_compile_error: import() cannot be used with 'new'
// skip-typecheck
// An ImportCall is not a MemberExpression, so it cannot be a new target.
new import("./x.js");

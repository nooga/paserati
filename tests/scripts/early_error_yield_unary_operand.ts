// expect_compile_error: Yield expression not allowed in this context
// YieldExpression is an AssignmentExpression, so it cannot be the operand of a
// unary operator without parentheses.
function* g() {
  void yield;
}

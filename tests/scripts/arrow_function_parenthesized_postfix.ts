// expect: 3
// An ArrowFunction is not a MemberExpression/LeftHandSideExpression, so `.`,
// `[`, `(`, etc. cannot apply to it directly - see
// class_computed_field_arrow_asi.ts for the paserati#292 bug this guards
// against. But once it's wrapped in its own parens, it's an ordinary
// PrimaryExpression again and postfix operators must keep working on it:
// `((x) => [x])(3)[0]` is valid, unambiguous JS with or without a newline in
// between. This is the positive counterpart that exercises
// ArrowFunctionLiteral.Parenthesized (parseGroupedExpression sets it,
// parseInfixContinuation's restriction checks it) - without it, the guard
// added for #292 would also wrongly reject this.
((x) => [x])(3)[0];

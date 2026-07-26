// expect_compile_error: The value 'null' cannot be used here.
// TS18050: `null` and `undefined` written directly as operands of an
// arithmetic, bitwise or relational operator. Concatenation is exempt, so the
// `+` below with a string operand must stay clean.

declare var n: number;
declare var s: string;

var concat = null + s;
var exponent = null ** n;

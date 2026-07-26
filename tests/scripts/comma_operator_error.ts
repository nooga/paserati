// expect_compile_error: Left side of comma operator is unused and has no side effects.
// TS2695: a discarded left operand that cannot do anything. The call and the
// increment below are effectful, and `(0, f)` is the indirect-call idiom, so
// none of those may be reported.

declare var flag: boolean;
function effect(): boolean { return true; }
let counter = 1;

effect(), flag;
counter++, flag;
(0, effect)();
flag, flag;

// BigInt and Number mixing should produce errors

let bigVal = 10n;
let numVal = 5;

bigVal + numVal;
// expect_compile_error: Operator '+' cannot be applied to types 'bigint' and 'number'.
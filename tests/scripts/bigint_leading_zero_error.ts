// BigInt literals cannot be legacy-octal-like.
// expect_compile_error: BigInt literals cannot have a leading zero

let x = 07n;

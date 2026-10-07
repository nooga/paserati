// expect_compile_error: The operand of a 'delete' operator must be a property reference.
// Test delete on non-reference values (literals, expressions)
// At runtime the ECMAScript spec makes `delete <non-reference>` evaluate to true,
// but TypeScript rejects it statically (TS2703), so the type checker must too.

let a = delete 1;
let b = delete "string";
let c = delete true;
let d = delete (1 + 2);
let e = delete typeof "foo";

a + "-" + b + "-" + c + "-" + d + "-" + e

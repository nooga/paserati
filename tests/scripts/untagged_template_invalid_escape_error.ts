// Only tagged templates may contain invalid escapes (their cooked value is
// undefined); an untagged template with one is a SyntaxError.
// expect_compile_error: Hexadecimal digit expected

let s = `\unicode`;

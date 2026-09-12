// expect: true
// paserati#426 Symptom 2 root cause: an invalid member-access continuation
// whose right-hand token is exactly the reserved word `enum` (e.g. a bare
// `.enum` where a property/identifier was expected - real ajv@8.17.1's
// bundled meta-schema validator does this, generating
// `params:{allowedValues: .enum}`, likely from a noderati/paserati bug
// elsewhere that drops the intended left-hand operand) resynced the parser
// onto the bare `enum` token. parseEnumDeclarationStatement/
// parseConstEnumDeclarationStatement then failed to parse a declaration
// there and returned a literal Go `nil` typed as `*ExpressionStatement` -
// the classic Go "typed nil in interface" trap: once implicitly widened to
// the `Statement` interface by parseStatement()'s dispatch, `stmt != nil`
// checks upstream see a non-nil interface and proceed to dereference the
// nil concrete pointer, panicking ("invalid memory address or nil pointer
// dereference") deep inside parseBlockStatement's hoisting check. This
// happened to require a genuinely deep enclosing nesting (~37+ levels) to
// manifest in the wild, but is fully independent of paserati#426 Symptom
// 1's own register-exhaustion bug - reproduced here with trivial nesting.
//
// Fixed by making both enum-declaration-statement parsers return a non-nil
// empty ExpressionStatement on failure (matching the identical defensive
// pattern already used by parseFunctionDeclarationStatement/
// parseAsyncFunctionDeclarationStatement a few hundred lines up in
// parser.go, for the exact same reason), so a failed parse is a normal
// compile/syntax error, not an uncaught panic - regardless of how deeply
// nested the surrounding code is.
let depth = 40;
let body = "";
let close = "";
for (let i = 0; i < depth; i++) {
  body += "if (true) {\n";
  close += "}\n";
}
body += "const err = {a: .enum};\n" + close;

let caught = false;
try {
  new Function(body);
} catch (e) {
  caught = e instanceof SyntaxError;
}

caught;

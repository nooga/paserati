// Unlike a for-loop head (see using_declaration_pattern_error.ts), a plain
// statement never forces `using` to be a declaration keyword: nothing rules
// out `using[a] = null` being an ordinary index-assignment expression on a
// variable named `using`. TypeScript parses `using [a] = null;` that way,
// so this reports plain "Cannot find name" errors rather than TS1492.

{
  using [a] = null;
}

// expect_compile_error: Cannot find name 'using'.

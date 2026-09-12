// paserati#433: helper module that deliberately fails to parse. No `// expect`
// comment, so scripts_test.go skips it as its own test (comment-scanning
// happens before any parse is attempted); it exists only to be re-exported
// by export_star_broken_source_helper_mid.ts.
export const a = 1;
this is not valid syntax @#$

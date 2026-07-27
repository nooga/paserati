// expect_compile_error: 'static' is a reserved word in strict mode. Class definitions are automatically in strict mode.
// TS1213: class bodies are strict whether or not --alwaysStrict is on, so a
// future reserved word used as a name there is an error. Outside a class
// Paserati runs sloppy mode, where these are ordinary identifiers — hence the
// companion runtime tests (yield_as_param_nonstrict.ts, let_as_identifier.ts)
// that must stay clean.

class C {
    m(): number {
        return static;
    }
}

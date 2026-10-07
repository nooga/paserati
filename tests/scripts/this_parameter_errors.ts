// An explicit 'this' parameter without a type annotation parses (tsc accepts it
// syntactically and its checker reports an implicit-any parameter, TS7006;
// FIXME: the implicit-any diagnostic for 'this' is not reported by our checker yet).
// expect: 1
function BadFunction1(this) {
    return 1;
}
BadFunction1.call({});

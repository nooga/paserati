// expect: true
// Found via a static "impossible condition: non-nil == nil" nilness-analyzer
// warning (golang.org/x/tools nilness, run over the whole repo while
// investigating paserati#426) - the exact same class of bug as that issue's
// root cause (parseEnumDeclarationStatement), just in a different spot.
//
// isGetterMethod()/isSetterMethod() (parser.go) treat a BIGINT peek token as
// "yes, this is a getter/setter" - but parseGetter/parseSetter's class-body
// parsing (parse_class.go) has no explicit BIGINT case before falling to
// the catch-all `else { propertyName = p.parsePropertyName(); if
// propertyName == nil { ... } }`, and parsePropertyName() itself has no
// BIGINT case either, so it returns a literal Go nil *Identifier there.
// Assigning that nil *Identifier into the `propertyName` Expression
// interface variable produces a non-nil interface wrapping a nil pointer
// (Go's classic typed-nil-in-interface trap), so the `== nil` check right
// after it was always false - dead code - and something downstream
// eventually dereferenced the nil concrete pointer, panicking.
//
// (The same fix was also applied to parseSetter, and to the analogous
// inline object-literal getter/setter parsing in parser.go, found by the
// same nilness pass - those two are defensive, since object-literal
// getters/setters already special-case BIGINT with parseBigIntLiteral()
// before ever reaching parsePropertyName(), so they weren't confirmed
// reachable the way the class-body cases below are.)
//
// Confirmed via `class A { get 123n() {} }` (a BigInt-literal property
// name): this genuinely reached parsePropertyName's nil-returning default
// case and crashed with "invalid memory address or nil pointer dereference"
// before this fix (contained as an opaque "internal compiler error" by
// paserati#426's separate recover() fix, but still not a specific, correct
// error). Fixed by checking the concrete *Identifier for nil before
// assigning it into the interface variable.
//
// Real Node actually accepts a BigInt-literal getter/setter name in a class
// body (unlike this repro) - that's a separate, minor, pre-existing feature
// gap, not fixed here; this test only asserts the crash is gone in favor of
// the correct clean, specific SyntaxError paserati already produces for the
// unsupported case.
const cases = ["class A { get 123n() {} }", "class A { set 123n(v) {} }"];

let allCaughtCleanly = true;
for (const src of cases) {
  try {
    new Function(src);
    allCaughtCleanly = false; // paserati doesn't support this shape (yet) - it must error, not silently succeed
  } catch (e) {
    if (!(e instanceof SyntaxError) || (e as Error).message.includes("internal compiler error")) {
      allCaughtCleanly = false;
    }
  }
}

allCaughtCleanly;

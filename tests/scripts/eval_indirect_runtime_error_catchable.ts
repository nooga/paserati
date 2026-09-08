// expect: ReferenceError:true:true:after:TypeError:true:undefined:true:object:true
// no-typecheck
// A runtime exception (e.g. ReferenceError) thrown by code passed to indirect
// eval() must be catchable by a try/catch surrounding the eval() call, exactly
// like a parse-time SyntaxError from indirect eval already was. It used to
// escape and terminate the whole script as an uncaught exception instead -
// vm.Interpret's nested run() stopped unwinding at the eval script's own
// top-level frame (a native-call boundary) without ever surfacing that state
// to the eval() native function, which then reported success. It also used to
// get rewrapped as a plain SyntaxError on the way out, discarding the real
// error type/instance.
var indirectEval = eval;

let caughtName = "no-throw";
let isReferenceError = false;
try {
  indirectEval("undefinedVar1234;");
} catch (e) {
  caughtName = e.name;
  isReferenceError =
    e instanceof ReferenceError && e.message === "undefinedVar1234 is not defined";
}

// Execution must continue normally after the catch - this used to never run.
let ranAfterCatch = false;
let afterMarker = "before";
try {
  indirectEval("(0, 0);");
  ranAfterCatch = true;
  afterMarker = "after";
} catch (e) {
  afterMarker = "unexpected-throw";
}

// A different runtime exception type must also come through unwrapped.
let caughtName2 = "no-throw";
let isTypeError = false;
try {
  indirectEval("null.foo;");
} catch (e) {
  caughtName2 = e.name;
  isTypeError = e instanceof TypeError;
}

// A thrown primitive that is itself falsy/nullish (as opposed to a genuine
// parse/compile failure) must also come through as the exact value thrown,
// not get miscategorized as "no wrapped exception, must be a SyntaxError"
// because GetExceptionValue() happens to be Undefined/Null too.
let undefThrowType = "no-throw";
let undefThrowIsUndefined = false;
try {
  indirectEval("throw undefined;");
} catch (e) {
  undefThrowType = typeof e;
  undefThrowIsUndefined = e === undefined;
}

let nullThrowType = "no-throw";
let nullThrowIsNull = false;
try {
  indirectEval("throw null;");
} catch (e) {
  nullThrowType = typeof e;
  nullThrowIsNull = e === null;
}

caughtName +
  ":" +
  isReferenceError +
  ":" +
  ranAfterCatch +
  ":" +
  afterMarker +
  ":" +
  caughtName2 +
  ":" +
  isTypeError +
  ":" +
  undefThrowType +
  ":" +
  undefThrowIsUndefined +
  ":" +
  nullThrowType +
  ":" +
  nullThrowIsNull;

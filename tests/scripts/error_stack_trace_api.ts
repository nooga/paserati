// expect: true
// V8 Stack Trace API (#492): Error.stackTraceLimit / Error.prepareStackTrace
// / structured CallSite objects. Real code (typescript-eslint among others)
// sets Error.prepareStackTrace to a custom formatter and expects
// Error.captureStackTrace / `new Error()` to hand it an array of CallSite
// objects instead of the usual formatted string.

// Default: a real number, per V8's default of 10.
const defaultLimitIsTen = Error.stackTraceLimit === 10;

// Without prepareStackTrace set, .stack stays the familiar formatted string.
const plainStack = new Error("plain").stack;
const plainStackIsString = typeof plainStack === "string";

// Installing prepareStackTrace redirects both `new Error()` and
// Error.captureStackTrace to hand back whatever it returns - here, the raw
// array of CallSite objects - instead of a string.
let capturedThis: any = undefined;
Error.prepareStackTrace = (err: any, stack: any) => {
  capturedThis = err;
  return stack;
};

const err = new Error("boom");
const stackIsArray = Array.isArray(err.stack);
const gotErrorBack = capturedThis === err;

const callSite = (err.stack as any)[0];
const callSiteWorks =
  typeof callSite.getFunctionName === "function" &&
  typeof callSite.getFileName === "function" &&
  typeof callSite.getLineNumber === "function" &&
  typeof callSite.getColumnNumber === "function" &&
  typeof callSite.getLineNumber() === "number" &&
  typeof callSite.getColumnNumber() === "number";

// Error.captureStackTrace on a plain object also goes through the hook.
const target: any = {};
Error.captureStackTrace(target, undefined as any);
const captureStackTraceUsesHook = Array.isArray(target.stack);

// Restoring prepareStackTrace to undefined restores string formatting.
Error.prepareStackTrace = undefined as any;
const restoredStack = new Error("after").stack;
const restoredIsString = typeof restoredStack === "string";

// Error.stackTraceLimit actually caps how many frames get captured.
function level1() {
  const r = level2();
  return r;
}
function level2() {
  const r = level3();
  return r;
}
function level3() {
  return new Error("deep").stack as string;
}

Error.stackTraceLimit = 1;
const limitedStack = level1();
const limitedLineCount = limitedStack.split("\n").length;
Error.stackTraceLimit = 10;

// expect: true
defaultLimitIsTen &&
  plainStackIsString &&
  stackIsArray &&
  gotErrorBack &&
  callSiteWorks &&
  captureStackTraceUsesHook &&
  restoredIsString &&
  limitedLineCount === 1;

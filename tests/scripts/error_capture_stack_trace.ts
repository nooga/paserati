// expect: true
// Error.captureStackTrace (#263): V8/Node non-standard extension widely
// called unconditionally by real code in its own error-handling paths.
// It must exist, not throw, and actually set a stack string on the target.

// Present and callable, not just a stub type.
const hasCaptureStackTrace = typeof Error.captureStackTrace === "function";

const e = new Error("x");
const originalStack = e.stack;
function marker() {}
Error.captureStackTrace(e, marker);

// Should not throw, and should still leave a non-empty stack in place
// (marker was never called, so nothing should be trimmed away).
const stackStillPresent = typeof e.stack === "string" && e.stack.length > 0;

// Passing a target that isn't an object should throw a TypeError rather
// than silently doing nothing.
let threwOnBadTarget = false;
try {
  Error.captureStackTrace(42 as any);
} catch (err) {
  threwOnBadTarget = err instanceof TypeError;
}

// When constructorOpt genuinely is on the stack (the common real-world
// pattern: a subclass constructor excluding its own frame), that frame
// should be trimmed from the resulting trace.
class MyError extends Error {
  constructor(msg: string) {
    super(msg);
    Error.captureStackTrace(this, MyError);
  }
}
function makeMyError() {
  return new MyError("boom");
}
const myErr = makeMyError();
// The constructor's own frame ("at MyError (...)") must be trimmed away;
// "makeMyError"'s frame (which legitimately contains "MyError" as a
// substring) must remain, so check for the exact frame text instead of a
// loose substring match.
const ownFrameTrimmed = !myErr.stack!.includes("at MyError (");

hasCaptureStackTrace &&
  typeof originalStack === "string" &&
  stackStillPresent &&
  threwOnBadTarget &&
  ownFrameTrimmed;

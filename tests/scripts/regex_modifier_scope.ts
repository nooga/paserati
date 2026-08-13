// Inline `(?s:)` / `(?-s:)` modifiers scope dotAll to their own group.
//
// The rewrite that gives `.` its ECMAScript meaning used to be skipped entirely
// whenever any inline `s` modifier appeared, so a `.` OUTSIDE the group fell
// back to Go's `.` and matched \r, U+2028 and U+2029 — the characters the
// rewrite exists to exclude. Newlines were never the symptom, which is why this
// covers all three.

const LS = "\u2028";
const PS = "\u2029";

// A `.` outside the modified group stays narrow.
const outerCR = /(?s:x)a.b/.test("xa\rb") === false;
const outerLS = /(?s:x)a.b/.test("xa" + LS + "b") === false;
const outerPS = /(?s:x)a.b/.test("xa" + PS + "b") === false;

// ...while the one inside it matches a line terminator, which is the point.
const innerNL = /(?s:a.)/.test("a\n") === true;

// No modifier anywhere: unchanged behaviour, and the control for the above.
const plainCR = /a.b/.test("a\rb") === false;
const plainNL = /a.b/.test("a\nb") === false;

// `(?-s:)` removes dotAll under an s flag, and the spec's own nesting example
// (built-ins/RegExp/regexp-modifiers/remove-dotAll.js) reads both directions:
// outside the group `.` spans newlines, inside it must not.
const removed = /(?-s:a.)/s.test("a\nb") === false;
const nestOutside = /a.(?-s:b.b).c/s.test("a\nb,b\nc") === true;
const nestInside = /a.(?-s:b.b).c/s.test("a,b\nb,c") === false;

outerCR && outerLS && outerPS && innerNL && plainCR && plainNL &&
  removed && nestOutside && nestInside;

// expect: true

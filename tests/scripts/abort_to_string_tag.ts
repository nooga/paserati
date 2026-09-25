// AbortSignal and AbortController carry Symbol.toStringTag (#567).
// expect: [object AbortSignal] [object AbortController]
const ac = new AbortController();
String(ac.signal) + " " + String(ac);
